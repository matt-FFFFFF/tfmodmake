package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

// UpdateResult holds the outcome of an update operation.
type UpdateResult struct {
	OldVersion         string
	NewVersion         string
	Variables          UpdateSummary
	Locals             UpdateSummary
	MainUpdated        bool
	OutputsRegenerated bool
}

// UpdateSummary classifies the changes made to a set of named items (variables or locals).
type UpdateSummary struct {
	AutoUpdated []string // names of items auto-updated
	Added       []string // names of new items added
	Removed     []string // names of items removed
	NeedsReview []string // names of user-modified items requiring manual attention
	Unchanged   []string // names of items identical between old and new spec
}

// UpdateOptions configures how the update is performed.
type UpdateOptions struct {
	// ModuleDir is the directory containing the existing module.
	ModuleDir string
	// OldSpecs is a list of paths or URLs to the old (current) OpenAPI spec files.
	// Used to generate the baseline for 3-way comparison.
	OldSpecs []string
	// NewSpecs is a list of paths or URLs to the new OpenAPI spec files.
	NewSpecs []string
	// ResourceType overrides the resource type (if empty, inferred from main.tf).
	ResourceType string
	// LocalName overrides the local variable name (default: "resource_body").
	LocalName string
	// DryRun, when true, computes changes without writing to disk.
	DryRun bool
}

// Update upgrades an existing Terraform module to a new API version while preserving
// user customizations. It performs a 3-way comparison:
//  1. Baseline: generated from old (current) spec
//  2. On-disk: the user's actual files (may include customizations)
//  3. New: generated from the new spec
//
// Items where on-disk matches baseline are auto-upgraded to new. Items where on-disk
// differs from baseline are flagged for manual review.
func Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	if opts.ModuleDir == "" {
		opts.ModuleDir = "."
	}
	if opts.LocalName == "" {
		opts.LocalName = "resource_body"
	}

	// Step 1: Read current module state from disk.
	mainFile, err := ParseModuleFile(opts.ModuleDir, "main.tf")
	if err != nil {
		return nil, fmt.Errorf("reading main.tf: %w", err)
	}
	resourceType, oldVersion, err := ExtractResourceTypeAndVersion(mainFile)
	if err != nil {
		return nil, fmt.Errorf("extracting resource type and version: %w", err)
	}
	if opts.ResourceType != "" {
		resourceType = opts.ResourceType
	}

	varsFile, err := ParseModuleFile(opts.ModuleDir, "variables.tf")
	if err != nil {
		return nil, fmt.Errorf("reading variables.tf: %w", err)
	}
	onDiskVarTypes := ExtractVariableTypes(varsFile)

	var onDiskLocalAssignments map[string]hclwrite.Tokens
	localsPath := filepath.Join(opts.ModuleDir, "locals.tf")
	var localsFile *hclwrite.File
	if _, statErr := os.Stat(localsPath); statErr == nil {
		localsFile, err = ParseHCLFile(localsPath)
		if err != nil {
			return nil, fmt.Errorf("reading locals.tf: %w", err)
		}
		onDiskLocalAssignments = ExtractLocalAssignments(localsFile)
	}

	// Step 2: Generate baseline from old (current) spec for dirty detection.
	baselineResult, err := LoadResource(ctx, opts.OldSpecs, resourceType)
	if err != nil {
		return nil, fmt.Errorf("loading resource from old specs: %w", err)
	}
	baselineModule, err := GenerateInMemory(resourceType,
		baselineResult,
		WithLocalName(opts.LocalName),
	)
	if err != nil {
		return nil, fmt.Errorf("generating baseline module: %w", err)
	}
	baselineVarTypes := ExtractVariableTypes(baselineModule.Variables)
	var baselineLocalAssignments map[string]hclwrite.Tokens
	if baselineModule.Locals != nil {
		baselineLocalAssignments = ExtractLocalAssignments(baselineModule.Locals)
	}

	// Step 3: Generate new module from new spec.
	newResult, err := LoadResource(ctx, opts.NewSpecs, resourceType)
	if err != nil {
		return nil, fmt.Errorf("loading resource from new specs: %w", err)
	}
	newModule, err := GenerateInMemory(resourceType,
		newResult,
		WithLocalName(opts.LocalName),
	)
	if err != nil {
		return nil, fmt.Errorf("generating new module: %w", err)
	}

	newVarTypes := ExtractVariableTypes(newModule.Variables)
	_, newVersion, err := ExtractResourceTypeAndVersion(newModule.Main)
	if err != nil {
		return nil, fmt.Errorf("extracting new version: %w", err)
	}

	var newLocalAssignments map[string]hclwrite.Tokens
	if newModule.Locals != nil {
		newLocalAssignments = ExtractLocalAssignments(newModule.Locals)
	}

	result := &UpdateResult{
		OldVersion: oldVersion,
		NewVersion: newVersion,
	}

	// Step 4: 3-way comparison and apply changes.
	// Compare on-disk against baseline to detect user modifications, then apply
	// new spec changes only to unmodified items.
	varComparison := CompareVariables(onDiskVarTypes, baselineVarTypes, newVarTypes)
	localComparison := CompareLocals(onDiskLocalAssignments, baselineLocalAssignments, newLocalAssignments)

	if !opts.DryRun {
		// Update variables.tf
		result.Variables = applyVariableChanges(varsFile, newModule.Variables, newVarTypes, varComparison)

		// Update locals.tf — create the file if it doesn't exist but the new spec needs one.
		if newModule.Locals != nil {
			if localsFile == nil {
				localsFile = hclwrite.NewEmptyFile()
				localsFile.Body().AppendNewBlock("locals", nil)
			}
			result.Locals = applyLocalChanges(localsFile, newModule.Locals, newLocalAssignments, localComparison)
		}

		// Update main.tf: type attribute and response_export_values
		newTypeString := resourceType + "@" + newVersion
		if err := UpdateResourceTypeAttribute(mainFile, newTypeString); err != nil {
			return nil, fmt.Errorf("updating type attribute: %w", err)
		}
		result.MainUpdated = true

		// Write updated files
		if err := writeHCLFile(filepath.Join(opts.ModuleDir, "variables.tf"), varsFile); err != nil {
			return nil, fmt.Errorf("writing variables.tf: %w", err)
		}
		if err := writeHCLFile(filepath.Join(opts.ModuleDir, "main.tf"), mainFile); err != nil {
			return nil, fmt.Errorf("writing main.tf: %w", err)
		}
		if localsFile != nil {
			if err := writeHCLFile(filepath.Join(opts.ModuleDir, "locals.tf"), localsFile); err != nil {
				return nil, fmt.Errorf("writing locals.tf: %w", err)
			}
		}

		// Regenerate outputs.tf from new spec
		if newModule.Outputs != nil {
			if err := writeHCLFile(filepath.Join(opts.ModuleDir, "outputs.tf"), newModule.Outputs); err != nil {
				return nil, fmt.Errorf("writing outputs.tf: %w", err)
			}
			result.OutputsRegenerated = true
		}
	} else {
		// Dry run: compute what would change using the 3-way comparison.
		result.Variables = summarizeComparison(varComparison)
		result.Locals = summarizeComparison(localComparison)
		result.MainUpdated = oldVersion != newVersion
		result.OutputsRegenerated = true
	}

	return result, nil
}

// applyVariableChanges modifies the on-disk variables file based on the 3-way comparison results.
func applyVariableChanges(diskFile, newFile *hclwrite.File, newTypes map[string]hclwrite.Tokens, comparison map[string]CompareResult) UpdateSummary {
	var summary UpdateSummary

	for name, cmp := range comparison {
		switch cmp {
		case CompareIdentical:
			// On-disk matches baseline; update to new spec type.
			newTokens, ok := newTypes[name]
			if !ok {
				continue
			}
			if err := UpdateVariableType(diskFile, name, newTokens); err != nil {
				summary.NeedsReview = append(summary.NeedsReview, name+" (update failed: "+err.Error()+")")
				continue
			}
			summary.AutoUpdated = append(summary.AutoUpdated, name)

		case CompareModified:
			// User has customized this variable — flag for manual review.
			summary.NeedsReview = append(summary.NeedsReview, name)

		case CompareNew:
			// New variable in the new spec — add it.
			if err := AddVariableBlock(diskFile, newFile, name); err != nil {
				summary.NeedsReview = append(summary.NeedsReview, name+" (add failed: "+err.Error()+")")
				continue
			}
			summary.Added = append(summary.Added, name)

		case CompareRemoved:
			// Variable removed in the new spec — remove it.
			if err := RemoveVariableBlock(diskFile, name); err != nil {
				summary.NeedsReview = append(summary.NeedsReview, name+" (remove failed: "+err.Error()+")")
				continue
			}
			summary.Removed = append(summary.Removed, name)
		}
	}

	return summary
}

// applyLocalChanges modifies the on-disk locals file based on the 3-way comparison results.
func applyLocalChanges(diskFile, newFile *hclwrite.File, newLocals map[string]hclwrite.Tokens, comparison map[string]CompareResult) UpdateSummary {
	var summary UpdateSummary

	newLocalAssignments := ExtractLocalAssignments(newFile)

	for name, cmp := range comparison {
		switch cmp {
		case CompareIdentical:
			newTokens, ok := newLocals[name]
			if !ok {
				continue
			}
			if err := UpdateLocalAttribute(diskFile, name, newTokens); err != nil {
				summary.NeedsReview = append(summary.NeedsReview, name+" (update failed: "+err.Error()+")")
				continue
			}
			summary.AutoUpdated = append(summary.AutoUpdated, name)

		case CompareModified:
			summary.NeedsReview = append(summary.NeedsReview, name)

		case CompareNew:
			tokens, ok := newLocalAssignments[name]
			if !ok {
				continue
			}
			if err := AddLocalAttribute(diskFile, name, tokens); err != nil {
				summary.NeedsReview = append(summary.NeedsReview, name+" (add failed: "+err.Error()+")")
				continue
			}
			summary.Added = append(summary.Added, name)

		case CompareRemoved:
			if err := RemoveLocalAttribute(diskFile, name); err != nil {
				summary.NeedsReview = append(summary.NeedsReview, name+" (remove failed: "+err.Error()+")")
				continue
			}
			summary.Removed = append(summary.Removed, name)
		}
	}

	return summary
}

// summarizeComparison converts a comparison map to an UpdateSummary for dry-run mode.
func summarizeComparison(comparison map[string]CompareResult) UpdateSummary {
	var summary UpdateSummary
	for name, cmp := range comparison {
		switch cmp {
		case CompareIdentical:
			summary.AutoUpdated = append(summary.AutoUpdated, name)
		case CompareModified:
			summary.NeedsReview = append(summary.NeedsReview, name)
		case CompareNew:
			summary.Added = append(summary.Added, name)
		case CompareRemoved:
			summary.Removed = append(summary.Removed, name)
		}
	}
	return summary
}

// writeHCLFile writes a parsed HCL file back to disk.
func writeHCLFile(path string, file *hclwrite.File) error {
	return os.WriteFile(path, file.Bytes(), 0o644)
}
