---
name: launch-customer-api
description: "Launch readiness step 5 — set up the API for customers. Makes the public API contract-first and safe to expose: an OpenAPI spec as the source of truth, versioning and deprecation policy, authentication and per-client scopes, rate limits and quotas, idempotency, consistent errors and pagination, gateway config, contract tests and customer-facing docs. Built into the orchestrator."
---

# launch-customer-api — set up the API for customers

Goal: an external customer can integrate from the docs alone, cannot hurt other customers or the
service, and will not be broken by the next release. Customers build on what you publish, so every
choice here is hard to undo — prefer the conventions the organization's other public APIs already
use (look for existing OpenAPI files, gateway configs such as Kong, Apigee, AWS API Gateway, Nginx,
Envoy, and API guidelines in the repositories) over inventing new ones.

## 1. Contract first

- An **OpenAPI 3.x** spec in the repository (`api/openapi.yaml` or the existing location) is the
  source of truth: every public operation, request/response schema, error, auth scheme and example.
- Generate or validate code against it (request validation middleware, or a CI check that the
  implementation and spec agree). Lint the spec (Spectral or the linter the repo uses).
- Mark internal endpoints so they are not exposed through the gateway.

## 2. Design rules

- **Versioning**: a major version in the path (`/v1/`) or header — one scheme, documented. Additive
  changes only within a version; breaking changes need a new version and a deprecation window
  (`Deprecation` and `Sunset` headers, changelog entry).
- **Errors**: one shape everywhere (RFC 9457 `application/problem+json` or the organization's
  existing format) with a stable machine-readable `code`, a human message, and a `request_id`.
  Never leak stack traces, SQL or internal hostnames.
- **Pagination**: cursor-based for lists that grow; a maximum page size.
- **Idempotency**: `Idempotency-Key` header on POSTs that create or charge; store and replay the
  first response for the key's lifetime.
- **Timestamps** in UTC ISO-8601; money as integer minor units plus a currency code.
- **Personal data**: return only the fields a client needs; mask sensitive identifiers by default.

## 3. Protect the service and other customers

- **Authentication**: OAuth 2.0 client credentials or API keys, per customer, rotatable without
  downtime; keys stored hashed; scopes per operation.
- **Authorization**: every request is scoped to the caller's tenant.
- **Rate limits and quotas** per client (and a global limit), returned as `429` with `Retry-After`
  and rate-limit headers. Values are ASSUMPTIONS until the operator sets them.
- **Payload limits**, request timeouts, and CORS only for browser clients that need it.
- **Audit log** of who called what (without request bodies holding personal data).

## 4. Prove it

- **Contract tests**: the implementation matches the spec (schemathesis, Dredd, Pact provider
  verification, or the repo's existing approach), run in CI.
- Tests for: auth failure (401/403), cross-tenant access denied, rate limit (429), idempotent
  replay, validation errors in the standard error shape.

## 5. Customer-facing docs

- Rendered reference from the spec (Redoc / Swagger UI / the portal the organization uses),
  getting-started guide (get credentials → first call → handle errors), auth guide, rate-limit and
  versioning policy, changelog, a sandbox/staging base URL, and support contact.
- A Postman collection or equivalent generated from the spec if the team uses one.

## Tools

| Tool | Need | Used for |
|---|---|---|
| `spectral` | **required** | lint the OpenAPI contract (`spectral lint api/openapi.yaml`) |
| `node` | recommended | npx-based contract-test and docs tooling (schemathesis also runs from Docker or pip) |

Install from the orchestrator's Tool Hub (**Launch Readiness** → *Install all missing*), the install prompt shown when the stage starts, or `go run ./cmd/daemon packages setup launch-readiness`. Never install software yourself from inside a stage: if a tool is missing, still wire it into the pipeline, give the exact command, and list it under operator actions.

## Output

```
## API surface
(operation — method path — auth scope — rate limit — idempotent?)
## Contract & validation
## Protection
(auth, tenancy, limits, gateway config — file per item)
## Tests added
## Customer docs
## Operator actions
(gateway/portal setup, client onboarding, limits to confirm)
## Summary
```
