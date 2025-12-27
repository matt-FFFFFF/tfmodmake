# REST API Spec Issues (Azure)

This document captures inconsistencies observed in Azure REST API specifications (both Swagger/OpenAPI v2 and OpenAPI v3) that impact `tfmodmake` generation.

The goal is not to "blame" upstream specs, but to record:

- What patterns exist in the wild
- How they break naïve tooling
- What mitigations `tfmodmake` implements
- What we consider a smell / tech debt

## Swagger / OpenAPI v2 Body Parameters ("in: body")

### The issue

Older Swagger/OpenAPI v2 specs model request bodies as a **parameter**:

- `parameters: [{ in: "body", schema: ... }]`

Rather than OpenAPI v3's `requestBody` block.

### Impact

If tooling only reads OpenAPI v3 `requestBody`, it may fail to find the PUT body schema and cannot generate Terraform inputs.

### Mitigation in `tfmodmake`

`tfmodmake` supports a v2-style fallback when identifying the PUT schema:

- Prefer OpenAPI v3 `requestBody`
- Else scan PUT `parameters` for a `body` parameter with a schema

This compatibility is required for consuming Azure specs in the wild.

## Secrets Not Marked With `x-ms-secret` (Description-Based Detection)

### The issue

`tfmodmake`'s primary secret signal is the Azure extension:

- `x-ms-secret: true`

In some specs, sensitive fields are **not marked with** `x-ms-secret`, even though they behave like secrets.

Example (Key Vault ARM secrets): `properties.value`

- Is accepted on PUT/PATCH
- Is never returned from the service
- Is highly sensitive

But the schema may only express this through the **description** (e.g. "will never be returned") rather than a machine-readable extension.

### Why this is a smell

Using natural-language text as a signal is fragile:

- descriptions vary by wording and capitalization
- translations / copy edits can break detection
- it can create false positives if phrasing appears in unrelated fields

This is explicitly considered a tooling smell.

### Mitigation in `tfmodmake`

`tfmodmake` uses a conservative secret-detection stack:

1. `writeOnly: true` (OpenAPI 3)
2. `x-ms-secret: true` (Azure extension)
3. **Exception:** description contains phrases like "will never be returned"

The description-based path is only intended to bridge real-world gaps in upstream specs.

### Known tradeoffs

- Description heuristics can be wrong.
- If this becomes a broader pattern, we should prefer an explicit allowlist/override mechanism over additional string-matching heuristics.

## Open Questions / Future Improvements

- Add an explicit generator override mechanism for secrets (e.g. a CLI flag or config file listing JSON paths treated as secrets).
- Add a per-service "spec quirk" registry keyed by resource type + api-version (prefer deterministic behavior over heuristics).
- Improve test coverage with more real-world specs that exhibit the above issues.