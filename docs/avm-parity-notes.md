# AVM Parity Notes (Spec-Driven, Generic)

## Purpose

tfmodmake’s generator is intentionally **generic**: it should produce a solid “starting module” from Azure REST API specs (OpenAPI), without embedding per-resource special cases.

This document captures what we learned by comparing:

- A **generated** module output (example: Container Apps Managed Environment from `gen avm`)
- A **hand-crafted** Azure Verified Module (AVM) for the same resource

…then generalizes into improvements that keep the tool spec-driven and transferable to other resources (e.g., Storage, AKS).

Scope note:

- This comparison focuses primarily on **Terraform configuration** (variables/locals/main/outputs, and generated child module wiring).
- It intentionally ignores ancillary module concerns such as documentation content, `terraform-docs`/README structure, TFLint config, CI scaffolding, example state files, and similar non-core artifacts.

Non-goals:

- “Match the hand-crafted AVM exactly”
- Add resource-specific idempotency hacks or conditional logic tailored to one service
- Telemetry parity (out of scope for this doc)

## Observed Differences (Generalized)

### 1) Inputs: spec-shaped vs opinionated UX

**Generated modules** tend to reflect the OpenAPI request payload shape closely:

- Nested objects and lists mirror schema structure
- Many properties are optional and default to `null`

**Hand-crafted AVMs** often provide an opinionated, ergonomically curated API:

- Inputs split into “simple knobs”
- Some fields are renamed or normalized
- Cross-field validation and behavior are encoded in Terraform

Generic takeaway:

- tfmodmake should remain **schema-first**, but can still offer optional generic UX layers that are *derived* from spec signals.

### 2) Computed outputs: “response fields” vs “composition outputs”

Generated modules often expose computed values directly from `response_export_values`.

Hand-crafted AVMs frequently also export *composition-friendly* outputs such as:

- Maps of child resource IDs keyed by input keys
- Structured “resource id maps” for submodules

Generic takeaway:

- When tfmodmake generates `for_each` child modules, it can also generate a **generic output map** (`local.<child>_resource_ids` + output) without any resource-specific knowledge.

### 3) Secrets: “field-level” vs “workflow-level” secret management

Specs can mark secret fields (`x-ms-secret`), which tfmodmake uses to:

- Exclude secrets from `body`
- Provide ephemeral variables
- Use `sensitive_body` + version mapping

Hand-crafted AVMs sometimes go further:

- Use `ephemeral azapi_resource_action` / lookup patterns to fetch secrets indirectly (e.g., shared keys)

Generic takeaway:

- Fetching derived secrets from other resources is powerful, but typically requires extra dependencies and assumptions.
- If we add such patterns, they should be **optional**, **generic**, and driven by discoverable spec signals (see “Candidate Enhancements”).

### 4) API versions: stable vs preview feature coverage

Hand-crafted modules may intentionally pin preview API versions to cover features absent in stable.

Generic takeaway:

- tfmodmake should expose a **generic mechanism** to prefer/allow preview API versions (already partly covered by `-include-preview` in discovery), and possibly allow an explicit override.

## Candidate Enhancements (Keep It Generic)

The goal here is “maximum information from specs” while staying generic.

### B) Generic “child module id map” outputs

When tfmodmake generates a child module with `for_each`, also generate:

- `local.<child>_resource_ids = { for k, v in module.<child> : k => { id = v.resource_id } }`
- Output `<child>_resource_ids` (map of objects)

This is generic across all resources and improves composition.

### D) Generic cross-field validation derived from schema

Some of the hand-crafted AVM validations are “semantic” rather than schema-driven.

But tfmodmake can still improve by generating more validations from schema data:

- `enum` validation (already done)
- `pattern` validation
- numeric bounds
- string length
- required relationships (e.g. when a nested object exists)

Reality check:

- tfmodmake already generates a fairly broad set of schema-derived validations today (enums including `x-ms-enum`, min/max length, regex `pattern`, UUID `format`, min/max items, unique items, numeric bounds and multipleOf). See [docs/validations.md](docs/validations.md).

Where we can potentially add more (still schema-bound):

- **Object size constraints (`minProperties` / `maxProperties`)**: `openapi3.Schema` (kin-openapi v0.133.0) exposes these as `MinProps` / `MaxProps`. However, in Terraform these constraints are only straightforward to validate for *map-like* objects (i.e. schemas using `additionalProperties`, which typically map to `map(...)` in Terraform). Fixed-shape `object({ ... })` values don’t have a reliable “count of present keys” primitive.
- **Additional string formats (opt-in)**: The schema `format` field sometimes contains `date-time`, `date`, `ipv4`, `ipv6`, `email`, etc. We can add an opt-in mode to validate these formats using conservative regexes, but keep the default minimal to avoid false positives.
- **Broader nested validations (opt-in)**: tfmodmake currently does nested validations conservatively (scalar fields + arrays of scalars) to avoid verbosity and complexity. An opt-in could expand nested validation coverage while still remaining schema-driven.

Principle:

- Only generate validations that can be derived directly from schema constraints; avoid semantic rules like “if internal LB enabled then force public access disabled” (service semantics).

## How to Evaluate Across Other Resources (Storage / AKS)

To ensure enhancements remain generic, evaluate them against multiple unrelated resources.

Suggested comparison checklist:

1. **Spec quality**
   - Is `x-ms-secret` present and accurate?
   - Are `readOnly` fields marked?
   - Are patterns/enum constraints present?

2. **Child resource patterns**
   - Do children follow consistent `/<parent>/<childType>` patterns?
   - Are there “child-like” concepts that are really interfaces (like private endpoints)?

3. **Identity / tags / location**
   - Are these exposed consistently?

4. **Secrets shape**
   - Are secrets scalar fields, nested fields, or array-item fields?

5. **Outputs usability**
   - Do consumers need ID maps for children?

A change is a good candidate for tfmodmake if it:

- Improves output quality for multiple resources
- Relies on spec signals (or is a generic opt-in feature)
- Does not require encoding service-specific semantic knowledge

## Recommendations (Priority)

1. Add generic output maps for child modules (composition-friendly)
2. Extend validations generation where additional schema constraints exist (still schema-bound)
