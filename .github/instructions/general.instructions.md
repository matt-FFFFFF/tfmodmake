
---
description: 'Instructions specific to this repository'
applyTo: '**'
---

## Repo Policy: No Backward Compatibility

- Assume this repo does **not** require backward compatibility for *its own public APIs/flags/output* unless the user explicitly asks for it.
- Prefer a single correct internal implementation over retaining legacy/alternate paths.
- If you introduce a replacement for existing behavior, **surgically remove** the old internal implementation (including deprecated functions, compatibility shims, and unused helpers).
- Avoid leaving “deprecated for compatibility” internal code behind; keep the codebase as clean and minimal as possible.

## External Compatibility (Upstream Inputs)

- Treat compatibility needed to consume *external inputs* (e.g., Azure REST API specs that may still be Swagger/OpenAPI v2 in places) as **required functionality**, not “legacy internal code”.
- Keep such external-compat paths when they are necessary to support real upstream artifacts; prefer narrow, well-tested fallbacks (e.g., Swagger 2 `parameters` with `in: body` as a fallback to OpenAPI 3 `requestBody`).
- If an external-compat path becomes unnecessary (no longer used by upstream inputs we care about), remove it and update tests/docs accordingly.