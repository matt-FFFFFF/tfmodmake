package terraform

import (
	"bytes"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// CompareResult classifies an item during update comparison.
type CompareResult int

const (
	// CompareIdentical means the on-disk item matches the baseline (auto-upgradable).
	CompareIdentical CompareResult = iota
	// CompareModified means the on-disk item differs from the baseline (user-modified).
	CompareModified
	// CompareNew means the item exists in the new spec but not the old.
	CompareNew
	// CompareRemoved means the item exists in the old spec but not the new.
	CompareRemoved
)

// NormalizeTokens strips whitespace, newlines, and comment tokens to enable
// format-insensitive comparison of HCL expressions.
func NormalizeTokens(tokens hclwrite.Tokens) hclwrite.Tokens {
	out := make(hclwrite.Tokens, 0, len(tokens))
	for _, tok := range tokens {
		switch tok.Type {
		case hclsyntax.TokenNewline, hclsyntax.TokenComment:
			continue
		}
		// Collapse whitespace in token bytes (spaces, tabs) but keep the token
		// itself since it may carry semantic content.
		cleaned := bytes.TrimSpace(tok.Bytes)
		if len(cleaned) == 0 {
			continue
		}
		out = append(out, &hclwrite.Token{
			Type:  tok.Type,
			Bytes: cleaned,
		})
	}
	return out
}

// TokensEqual compares two token sequences after normalization.
func TokensEqual(a, b hclwrite.Tokens) bool {
	na := NormalizeTokens(a)
	nb := NormalizeTokens(b)
	if len(na) != len(nb) {
		return false
	}
	for i := range na {
		if na[i].Type != nb[i].Type {
			return false
		}
		if !bytes.Equal(na[i].Bytes, nb[i].Bytes) {
			return false
		}
	}
	return true
}

// CompareVariables compares on-disk variable types against baseline generated types
// and returns per-variable comparison results. The newGenerated map provides the
// new spec's generated types for detecting additions and removals.
func CompareVariables(onDisk, baseline, newGenerated map[string]hclwrite.Tokens) map[string]CompareResult {
	result := make(map[string]CompareResult)

	// Check all items in the new spec
	for name := range newGenerated {
		baselineTokens, inBaseline := baseline[name]
		diskTokens, onDiskExists := onDisk[name]

		if !inBaseline {
			// New in the new spec
			result[name] = CompareNew
			continue
		}

		if !onDiskExists {
			// Was in baseline but user deleted it — treat as user-modified so we
			// don't re-add something they intentionally removed.
			result[name] = CompareModified
			continue
		}

		// Exists in both specs — check if on-disk matches baseline
		if TokensEqual(diskTokens, baselineTokens) {
			result[name] = CompareIdentical
		} else {
			result[name] = CompareModified
		}
	}

	// Check for items in baseline that are not in the new spec (removed)
	for name := range baseline {
		if _, inNew := newGenerated[name]; !inNew {
			result[name] = CompareRemoved
		}
	}

	return result
}

// CompareLocals compares on-disk local assignments against baseline generated locals
// and returns per-assignment comparison results.
func CompareLocals(onDisk, baseline, newGenerated map[string]hclwrite.Tokens) map[string]CompareResult {
	// Same logic as CompareVariables — both operate on name->tokens maps.
	return CompareVariables(onDisk, baseline, newGenerated)
}
