# Child resource discovery: why results can be incomplete

The `tfmodmake children` command is intended as a *helper* for module authors: it provides a “good enough” starting point for identifying which child resource types are plausibly in-scope under a given parent resource type.

It is **not** a guarantee of completeness or deployability across all Azure API versions.

## Why the initial approach can return only a subset

The initial implementation of `children` worked like this:

1. You pass one (or a few) OpenAPI/Swagger documents via `-spec`.
2. `tfmodmake` scans each document’s `paths`.
3. It infers child resource types under `-parent` (e.g. `Microsoft.App/managedEnvironments`).
4. It filters to “deployable” children (roughly: has a `PUT`/`PATCH` with a request body schema).

This can return only a subset of child resources because **Azure REST specs are frequently split across multiple files**.

### Example: Managed Environments

In `azure-rest-api-specs`, the Managed Environments surface is not always described by a single file.

For example, in some API-version folders you’ll see a set like:

- `ManagedEnvironments.json`
- `ManagedEnvironmentsDaprComponents.json`
- `ManagedEnvironmentsStorages.json`
- `ManagedEnvironmentsManagedCertificates.json`
- …etc

If you only pass `ManagedEnvironments.json`, the scan can only “see” the children described in that file. Any children that live in sibling files (like `.../daprComponents` or `.../storages`) will be missing.

This is a common pattern across Azure RPs: new feature areas, sub-resources, or extensions often get their own swagger file.

## Why we added GitHub directory-based discovery

To avoid requiring users to manually enumerate all sibling swagger files, `children` gained optional GitHub-based discovery.

### What discovery does

When enabled (`-discover` and/or `-spec-root`), `tfmodmake`:

- Uses the GitHub Contents API to list files in a directory.
- Filters filenames via a glob (for example `ManagedEnvironments*.json`).
- Downloads and scans *all* matching specs.

This makes results more complete for split spec sets and dramatically reduces manual effort.

### Deterministic “latest stable” selection

For module scoping, a common ask is “just give me a stable-by-default starting point.”

When you use `-spec-root`, discovery selects the latest date-style API-version folder under `stable/` deterministically.

Optionally also include the latest `preview/` folder with `-include-preview`.

This is deterministic and avoids the “which folder should I pick?” problem.

## Downsides and compromises

### 1) GitHub rate limiting

Directory discovery uses GitHub APIs. Unauthenticated requests can hit rate limits quickly.

- If you see rate limit errors, set `GITHUB_TOKEN` (or `GH_TOKEN`) and retry.

### 2) Heuristics and naming conventions

Discovery depends on filename patterns.

- The default behavior tries `ParentName*.json` first and then falls back to `*.json`.
- This is a compromise:
  - narrower patterns reduce unrelated noise and make results more deterministic
  - broader patterns are more robust, but can pull in unrelated specs from large folders

### 3) “Deployable” is a best-effort classification

`children`’s deployability filter is deliberately simple: it uses HTTP operations and the presence of a request body schema as a proxy.

This can produce surprising outcomes:

- Some resource types appear in documentation tooling as “deployable”, but their OpenAPI surface may be **GET-only** (read-only/effective views).
- Some resources are deployable but have unusual shapes that don’t look like a standard PUT-with-body.

For module scoping, the key idea is:

- “deployable child” is a *helpful signal*, not a contract.

### 4) Preview vs stable tradeoffs

Including preview can reveal newer child types earlier, but it has costs:

- preview APIs can change
- preview folders can contain additional split files
- preview can surface read-only “configuration” resources that are not truly user-creatable

For module scoping:

- stable-only is usually the safest default
- stable + latest preview is often the best “don’t miss things” mode

### 5) Completeness across versions is not guaranteed

Even with directory discovery, `children` only sees what is present in the chosen folder(s). Azure RPs may:

- add/remove child resources across API versions
- split/merge swagger files over time
- have different coverage in stable vs preview

If you need completeness across many versions, you generally need a broader crawl (and more complex “latest per child” selection logic).

## Recommended usage for module scoping

If you’re using `children` to identify resources in-scope for a module, a practical workflow is:

1. Start with latest stable (default behavior with `-spec-root`).
2. If you suspect the RP has newer surfaces, add latest preview (`-include-preview`).
3. Treat the output as a starting list; confirm edge cases with real provider behavior.

## Commands

Managed Environments (latest stable + preview):

```bash
./tfmodmake children \
  -spec-root "https://github.com/Azure/azure-rest-api-specs/tree/main/specification/app/resource-manager/Microsoft.App/ContainerApps" \
  -include-preview \
  -parent "Microsoft.App/managedEnvironments"
```

If GitHub directory listing is rate-limited, you can always fall back to explicit `-spec` URLs (more manual, but no directory API calls).
