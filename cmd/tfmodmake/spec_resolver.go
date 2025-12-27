package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// ResolvedSpec is a concrete spec source selected for analysis.
//
// NOTE: This intentionally lives in the cmd package (not internal) because it is
// discovery/CLI wiring, not OpenAPI analysis.
type ResolvedSpec struct {
	Source string
	Origin string
	Reason string
}

type ResolveRequest struct {
	Seeds []string

	GitHubServiceRoot string

	DiscoverFromSeed bool

	IncludeGlobs []string

	IncludePreview bool

	GitHubToken string
}

type ResolveResult struct {
	Specs    []ResolvedSpec
	Warnings []string
}

type SpecResolver interface {
	Resolve(ctx context.Context, req ResolveRequest) (ResolveResult, error)
}

type defaultSpecResolver struct{}

func (r defaultSpecResolver) Resolve(ctx context.Context, req ResolveRequest) (ResolveResult, error) {
	_ = ctx // reserved for future cancellation/timeouts

	out := ResolveResult{
		Specs:    make([]ResolvedSpec, 0, len(req.Seeds)),
		Warnings: nil,
	}

	for _, seed := range req.Seeds {
		out.Specs = append(out.Specs, ResolvedSpec{Source: seed, Origin: "seed"})
	}

	if req.GitHubServiceRoot != "" {
		loc, err := parseGitHubTreeDirURL(req.GitHubServiceRoot)
		if err != nil {
			return ResolveResult{}, fmt.Errorf("invalid -spec-root: %w", err)
		}

		// Deterministic by default: treat -spec-root as a service root and select the latest stable
		// version folder (optionally also include the latest preview folder).
		urls, err := discoverDeterministicSpecSetFromGitHubDir(nil, loc, req.IncludeGlobs, req.GitHubToken, deterministicDiscoveryOptions{
			IncludePreview: req.IncludePreview,
		})
		if err != nil {
			return ResolveResult{}, fmt.Errorf("failed to discover specs from -spec-root: %w", err)
		}
		for _, url := range urls {
			out.Specs = append(out.Specs, ResolvedSpec{Source: url, Origin: "spec-root"})
		}
	}

	if req.DiscoverFromSeed {
		for _, seed := range req.Seeds {
			if _, _, _, _, ok := parseRawGitHubFileURL(seed); !ok {
				continue
			}

			glob := "*.json"
			if len(req.IncludeGlobs) > 0 {
				glob = req.IncludeGlobs[0]
			}
			urls, err := discoverSiblingSpecsFromRawGitHubSpecURL(nil, seed, glob, req.GitHubToken)
			if err != nil {
				return ResolveResult{}, fmt.Errorf("failed to discover sibling specs from -spec %s: %w", seed, err)
			}
			for _, url := range urls {
				out.Specs = append(out.Specs, ResolvedSpec{Source: url, Origin: "discover"})
			}
			break // preserve current behavior: discover from first raw GitHub spec only
		}
	}

	out.Specs = dedupeResolvedSpecsPreserveOrder(out.Specs)
	return out, nil
}

func dedupeResolvedSpecsPreserveOrder(specs []ResolvedSpec) []ResolvedSpec {
	seen := make(map[string]struct{}, len(specs))
	out := make([]ResolvedSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Source == "" {
			continue
		}
		if _, ok := seen[spec.Source]; ok {
			continue
		}
		seen[spec.Source] = struct{}{}
		out = append(out, spec)
	}
	return out
}

func writeResolvedSpecs(w io.Writer, specs []ResolvedSpec) {
	fmt.Fprintln(w, "Resolved specs")
	for _, spec := range specs {
		if spec.Source == "" {
			continue
		}
		// Keep output stable and script-friendly: one spec per line.
		fmt.Fprintln(w, spec.Source)
	}
}

func githubTokenFromEnv() string {
	tok := os.Getenv("GITHUB_TOKEN")
	if tok == "" {
		tok = os.Getenv("GH_TOKEN")
	}
	return tok
}
