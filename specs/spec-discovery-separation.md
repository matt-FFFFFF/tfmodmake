# Spec: Separate Spec Discovery from Children Analysis (In-Memory Pipeline)

## Status
- Draft

## Problem Statement
The `children` command currently blends two distinct responsibilities:

1. **Spec discovery / resolution**
   - Determine which OpenAPI/Swagger files to scan given a single seed input.
   - Handle GitHub directory listing, stable/preview folder structures, version selection, include globs, rate limiting, and deterministic ordering.

2. **Spec analysis**
   - Parse OpenAPI/Swagger documents.
   - Infer ARM resource types from paths.
   - Determine parent/child relationships and deployability.
   - Format output.

Blending these concerns increases UX and debugging ambiguity:
- When output is missing a resource type, it’s unclear if the cause is discovery (wrong/missing inputs) or analysis (parser/inference bug).
- `children` accrues network-related flags and behaviors that are orthogonal to analysis.

This spec proposes a clean internal separation while keeping a single-binary workflow and avoiding user-managed intermediate files.

## Goals
- **Logical separation**: Discovery and analysis are separate modules/components with explicit contracts.
- **No required intermediate artifacts**: Resolved spec sets are passed in-memory by default.
- **Deterministic**: Given the same inputs and GitHub directory state, the resolved spec list is ordered deterministically.
- **Observable**: Users can optionally see what specs were resolved (without saving files).
- **Single-command workflow**: Keep discovery on `children` so you can go from “service root URL” to results in one command.
- **Testable**: Resolver behavior can be unit-tested without exercising the analyzer.

## Non-Goals
- Not redesigning deployability heuristics (PUT/PATCH + request body) in this spec.
- Not changing output format in this spec.
- Not implementing persistent caching here (optional future enhancement).

## Terminology
- **Seed spec**: A spec URL or local path passed by the user.
- **GitHub directory root**: A GitHub tree URL pointing at a folder, used as the basis for discovery.
- **Resolved spec set**: The ordered list of concrete spec URLs/paths that will be loaded and analyzed.
- **Analyzer**: The existing logic that loads and parses specs and yields child resource candidates.

## Architecture Overview
### High-level pipeline

```
Inputs (flags)
   │
   ├─ if discovery enabled: Resolver.Resolve(...) → ResolvedSpecSet
   │
   └─ else: user-provided -spec values → ResolvedSpecSet

ResolvedSpecSet
   │
Analyzer.Analyze(ResolvedSpecSet, parent, depth) → ChildrenResult
   │
Formatter → stdout
```

### Component boundaries

- **Resolver**: Responsible for producing a `ResolvedSpecSet`.
  - Inputs: seed URLs, GitHub service root, include globs, include-preview.
  - Output: ordered list of spec sources with metadata (where it came from, why selected).
  - Does not parse OpenAPI.

- **Analyzer**: Responsible for consuming a spec set and producing children.
  - Inputs: list of spec sources, parent type, depth.
  - Output: `ChildrenResult` (deployable + filtered out with reasons).
  - Should not know about GitHub directories or stable/preview selection.

- **Formatter**: Responsible for output only.

This separation ensures missing results can be diagnosed by inspecting the resolved spec set independently from analysis.

## Data Model
### ResolveRequest
A request for producing a resolved spec set.

Required fields:
- `Seeds []string`
  - List of initial spec sources (URLs or local paths).

Optional fields:
- `GitHubServiceRoot string`
  - A GitHub tree directory URL used as discovery root.

- `IncludeGlobs []string`
  - Glob patterns for matching spec file names in a GitHub directory listing.

- `IncludePreview bool`
  - Also include the latest preview API version folder (in addition to latest stable).

### ResolvedSpec
A single resolved spec source.

- `Source string`
  - URL or local path.

- `Origin string`
  - e.g. `"seed"`, `"spec-root"`, `"discover"`.

- `APIVersionHint string`
  - Extracted from URL/path if available (e.g. `2025-06-01`).

- `StabilityHint string`
  - `"stable" | "preview" | "unknown"`.

- `Reason string`
  - Short explanation like:
    - `"matched include glob ManagedEnvironments*.json"`
    - `"picked latest stable version folder 2025-06-01"`

### ResolveResult
- `Specs []ResolvedSpec`
  - Ordered deterministically.

- `Warnings []string`
  - Rate limit, token missing, include glob matched nothing (fallback applied), etc.

- `Errors []error`
  - Non-fatal errors if partial resolution is supported.

## CLI Behavior
### Existing behavior preserved
`children` continues to accept repeated `-spec` flags.

Discovery-related flags remain on `children` for convenience, but are implemented as:

1. Build `ResolveRequest`
2. Call `Resolver.Resolve`
3. Pass `ResolveResult.Specs[].Source` into the analyzer

### Observability: print resolved specs
Add a flag to `children`:
- `-print-resolved-specs`
  - If set, print the resolved spec list to stderr before analysis.
  - Include `Source` and optionally `Reason`.

Rationale:
- Enables debugging and reproducibility without forcing intermediate files.

### Exit codes
- If resolver fails to produce any specs → non-zero.
- If resolver produced specs but analyzer fails to load one spec:
  - Current behavior is to fail the command.
  - Optional future enhancement: allow partial analysis with warnings.

## Determinism Requirements
- GitHub directory listings must be sorted deterministically (already implemented in current discovery).
- “Latest version” selection must be deterministic:
  - Use date-prefix parsing for version folders.
  - Handle `-preview` suffix.

## Testing Strategy
### Resolver tests
- URL parsing:
  - GitHub tree dir URL parsing.
  - Raw GitHub file URL parsing.

- Version folder selection:
  - stable-only.
  - stable + preview.
  - prefer preview.

- Include patterns:
  - exact case and common casing mismatches.
  - fallback behavior when include matches nothing.

- Error formatting:
  - rate limiting message includes suggestion for `GITHUB_TOKEN`/`GH_TOKEN`.

Resolver tests should not load OpenAPI documents.

### Analyzer tests
- Provide a fixed list of spec sources (local test fixtures) and assert children results.
- Keep analyzer tests independent of network and resolver.

## Migration Plan
1. Introduce `resolver` package/module and move existing discovery logic into it.
2. Update `children` command implementation to call resolver when discovery flags are used.
3. Add `-print-resolved-specs` for debugging.
4. Update documentation:
   - Explain that discovery is a preparatory stage.
   - Describe how to debug missing children by printing resolved specs.

## Open Questions / Decisions

Resolved:
- `-print-resolved-specs` prints to **stderr** (diagnostic output; keep stdout clean for data output like `-json`).
- No public `specs` subcommand is planned; keep resolver functionality **internal** and exposed only via existing `children -discover*` flags plus observability.
- No hidden cache is planned; **token-based auth** is sufficient and avoids cache staleness/invalidations.

## Notes (Rationale)
- This separation preserves the original intent of `tfmodmake` (module-making) by treating spec discovery as a subordinate step in a module-scoping workflow.
- Making discovery observable without intermediate files reduces “messiness” while preserving debuggability.
