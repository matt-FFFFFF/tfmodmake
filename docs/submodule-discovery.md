# Submodule discovery from Azure REST (OpenAPI) specs

## Goal

Given a **parent resource type** (e.g. `Microsoft.App/managedEnvironments`) and one or more OpenAPI specs, infer a set of **candidate child resource types** that are likely to be modeled as Terraform “child submodules” (like the Managed Environment AVM’s certificates, managed certificates, dapr components, storages).

This is intended to power a future workflow where tfmodmake can:

1. discover child resource types under a parent (planning / inventory),
2. generate child modules for those resource types (azapi_resource scaffolds), and
3. optionally wire those modules into the parent module using the existing `addsub` wrapper mechanism.

Non-goals (at least initially):

- deciding module UX / schema beyond “candidate list + basic metadata”,
- generating opinionated AVM-level features (role assignments, diag settings, etc.) that are not ARM child resources.


## Why a separate subcommand (vs reusing `addsub`)

`addsub` today is intentionally narrow: it takes a **path to an existing Terraform module**, inspects its declared input variables, and emits wrapper plumbing (`variables.<name>.tf` and `main.<name>.tf`) to expose it as a map-of-objects `for_each` block.

Discovery is about answering a different question:

- “Given this parent ARM resource type, what child ARM resource types exist in the REST specs, and which look deployable?”

That question can’t be answered from Terraform module inputs alone, which is why overloading `addsub` would blur responsibilities.

Discovery is a different domain:

- it operates on **OpenAPI paths / operations** instead of HCL module inputs,
- it produces **candidate child resource types** (and metadata) rather than wrapper HCL,
- it needs filtering logic for **deployability/read-only** resources.

Recommendation: create a dedicated subcommand for discovery. Keep `addsub` as the wiring/wrapper generator.

Naming: `children` is a short, literal name for what it returns.

A future integrated flow can be:

- `children` → produces a machine-readable list (and a markdown summary)
- `generate` (existing) → can be run for each discovered child
- `addsub` (existing) → wires each generated child module into the parent


## CLI surface (proposed)

Keep the user-facing commands few and composable to reduce churn and merge conflicts.

### `children` (discovery)

Purpose: discover child resource types under a parent from one or more OpenAPI specs.

Example:

```bash
tfmodmake children \
  -spec https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironments.json \
  -parent Microsoft.App/managedEnvironments
```

Default output:

- Markdown summary to stdout (deployable + filtered out)
- Optional JSON output to stdout behind a flag (e.g. `-json`)

Notes:

- No intermediate file is required, but the implementation will necessarily build an in-memory model matching the “discovery spec” schema below.

### `addchild` (orchestration)

Purpose: take a specific child resource type (often chosen from `children` output) and integrate it into the module with minimal manual steps.

Scope (high-level):

- ensure a child module exists (generation step; future), then
- wire it into the root module (wrapper step via `addsub` behavior).

Example (shape only):

```bash
tfmodmake addchild \
  -spec <spec-url-or-path> \
  -parent Microsoft.App/managedEnvironments \
  -child Microsoft.App/managedEnvironments/certificates
```

### `addsub` (kept as-is)

Purpose: wrap an existing Terraform module directory into a map-of-objects `for_each` wrapper.

This remains useful independently (not just for ARM child resources) and is a stable building block for `addchild`.


## Core insight: ARM child resources are visible in spec paths

ARM instance resource paths typically look like:

- parent instance:
  - `.../providers/Microsoft.App/managedEnvironments/{environmentName}`
- child instance:
  - `.../providers/Microsoft.App/managedEnvironments/{environmentName}/certificates/{certificateName}`

If a path parses as a canonical ARM *instance path* (ending in `{name}` segments) we can derive a fully-qualified resource type like:

- `Microsoft.App/managedEnvironments`
- `Microsoft.App/managedEnvironments/certificates`
- `Microsoft.App/managedEnvironments/daprComponents`

This repo already contains a path parser that extracts this resource type from paths.


## Inputs

### Required

- `parent_resource_type`: string
  - Example: `Microsoft.App/managedEnvironments`

- `specs`: list of OpenAPI documents
  - May be:
    - URLs to raw azure-rest-api-specs JSON
    - local file paths

### Optional / recommended

- `provider_namespace`: string (redundant if present in `parent_resource_type`, but useful for guardrails)
  - Example: `Microsoft.App`

- `api_version_policy`: how to pick an api-version for each discovered child resource
  - default: `prefer_latest`
  - note: “prefer latest” needs a search space (either the supplied specs, or repo-wide). If only the supplied specs are used, the “latest” is “latest among those docs”.

- `discover_depth`: integer
  - default: `1` (direct children only)
  - allow `>1` if you want `parent/child/grandchild` discovery

- `require_instance_put_or_patch`: boolean
  - default: `true` (minimum deployability check)


## Discovery algorithm (high-level)

1. Load each OpenAPI spec document and resolve `$ref`s.
2. Walk all paths.
3. For each path:
   - If the path is a canonical ARM instance path:
     - derive `resource_type` (e.g. `Microsoft.App/managedEnvironments/certificates`)
     - derive `name_param` (e.g. `certificateName`)
   - Collect the operations present for that path (PUT/PATCH/GET/DELETE).
4. Normalize by `resource_type`:
   - keep a per-resource_type record of:
     - operations seen (instance operations)
     - candidate request schema(s) for PUT/PATCH
     - spec origins (which doc/path produced it)
5. Determine whether each `resource_type` is a child of the parent:
   - it is a child if:
     - it has the same provider namespace, and
     - it has prefix `${parent_resource_type}/`, and
     - the remaining suffix has exactly one segment when `discover_depth=1`
6. Apply “deployability” filtering (see below).


## Deployability / read-only filtering

Azure REST specs include many resources/paths that are not appropriate to generate as Terraform-managed resources.

### Definitions

- **Instance path**: a path that includes the full ARM hierarchy and ends in a `{name}` parameter for the resource instance.
- **Deployable candidate**: a resource type that appears to support create/update of an instance.

### Minimum viable deployability heuristic (first pass)

A discovered child resource type is considered deployable if all of the following are true:

1. An **instance path** exists for that resource type, and
2. the instance path supports **PUT** or **PATCH**, and
3. the PUT/PATCH operation has a **request body schema** (OpenAPI v3 RequestBody or swagger-v2 style body parameter).

Everything else is marked non-deployable and should be excluded by default or surfaced as “read-only candidate”.

Decision: include non-deployable entries in output (as “filtered out”) so the user can see what was considered and why.

This catches:

- list-only resources (GET collection)
- “status-only” resources (GET instance only)
- action endpoints (POST without instance semantics)

### Optional stronger heuristics

These are not required initially but can improve signal:

- Require DELETE to be present (some resources are create/update only; this may be too strict)
- Detect “read-only request schema”:
  - if the request schema exists but all properties are effectively readOnly / non-writable after applying writability overrides, treat as non-deployable
- Filter action-style endpoints:
  - paths like `.../{name}/listKeys`, `.../{name}/activate`, `.../{name}/regenerateKey` should not be treated as resources even though they may be POST


## Output: discovery spec (machine-readable)

### Top-level schema (JSON)

```json
{
  "parent_resource_type": "Microsoft.App/managedEnvironments",
  "generated_at": "2025-12-27T00:00:00Z",
  "inputs": {
    "specs": ["<url-or-path>", "<url-or-path>"]
  },
  "children": [
    {
      "resource_type": "Microsoft.App/managedEnvironments/certificates",
      "child_name_segment": "certificates",
      "depth": 1,
      "deployable": true,
      "deployability_reason": "PUT with request body found on instance path",
      "instance_operations": {
        "put": true,
        "patch": false,
        "get": true,
        "delete": true
      },
      "name_param": "certificateName",
      "parent_name_param": "environmentName",
      "path_examples": [
        "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/certificates/{certificateName}"
      ],
      "schema_hints": {
        "has_request_body": true,
        "request_body_media_types": ["application/json"]
      },
      "origins": [
        {
          "spec": "<url-or-path>",
          "path": "<path>",
          "operation": "put"
        }
      ]
    }
  ],
  "non_deployable": [
    {
      "resource_type": "Microsoft.App/managedEnvironments/someReadOnlyThing",
      "deployable": false,
      "deployability_reason": "GET-only instance path",
      "path_examples": ["..."]
    }
  ]
}
```

### Field definitions

- `parent_resource_type`: the resource type being analyzed.
- `children[]`: deployable child resource types (default output list).
- `non_deployable[]`: list of discovered child types that were filtered out.

Per-child fields:

- `resource_type`: fully qualified ARM resource type (provider + type segments)
- `child_name_segment`: the immediate child type segment (e.g. `certificates`)
- `depth`: integer depth below parent
- `deployable`: boolean
- `deployability_reason`: string explanation for filtering and debugging
- `instance_operations`: observed operations on an instance path
- `name_param`: name of the final instance name parameter
- `parent_name_param`: name of the parent’s instance parameter if detectable
- `path_examples`: concrete path templates observed
- `schema_hints`: small set of facts for downstream generation without embedding full schema
- `origins`: where the evidence came from (spec + path + operation)


## Output: human-readable report (markdown)

In addition to JSON, a markdown report is useful for review/triage.

Suggested sections:

- Parent type + list of analyzed specs
- “Deployable children” table
- “Non-deployable / read-only” table
- Notes about API version selection and ambiguities


## How this ties into generation + wiring

Once `children[]` is known:

- For each `children[i].resource_type`, run the existing generator to create a child module in a subfolder (future capability).
- Then run `addsub <child_module_path>` to generate wrapper wiring in the root module.

Important: child resources may have different api-versions than the parent.
Discovery should track api-version candidates per child type and let the caller choose the policy.


## Decisions (current)

1. Output includes filtered-out (non-deployable) results.
2. Deployability uses the minimum heuristic: instance path + PUT/PATCH + request body.
3. API version selection policy: prefer latest.

Implementation sequencing (to avoid merge conflicts):

- Prefer building `children` end-to-end first (data model + output formatting + filters) in a single PR.
- Only once that lands, layer `addchild` on top, reusing the same internal data model.


## Open questions

1. Search scope for “prefer latest”:
  - only within the provided `specs` (cheap, but “latest” is limited), or
  - support repo-wide search (more accurate, but requires access to a local clone/index of specs).
2. How to present results without an intermediate file:
  - print JSON to stdout + print markdown to stdout, or
  - print markdown by default with an option to emit JSON.
3. Should “deployable” ever require DELETE?
  - some ARM resources can be create/update without delete, so requiring DELETE may be too strict.

