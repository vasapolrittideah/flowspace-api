# ADR-0010: The public HTTP API is written specification-first

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `delivery`

## Context

FlowSpace has two API surfaces with different audiences. Services speak to each
other over Protobuf, where `buf breaking` already guards the contract
(ADR-0004). The public surface is REST over JSON, served by the BFF on chi, and
consumed by the web client and eventually by anything a customer scripts.

The public surface has no such guard, and it is the one where the consumer set
is unknown.

Documentation is intended to live in Postman, with Newman running a smoke suite
in the deployment pipeline. That is a good use of Newman and a bad use of
Postman: a hand-maintained collection has no mechanical link to the routes chi
actually serves. Within weeks there are endpoints missing from the collection
and collection requests aimed at endpoints that no longer exist, and nothing in
the system can detect either. The failure is not Postman's — it is having two
descriptions of one thing with nothing connecting them.

## Decision

`api/openapi.yaml` is the **single source of truth** for the public API. It is
written by hand, before the handler, and everything else is derived from it.

```text
api/openapi.yaml              hand-written, source of truth
  ├─ oapi-codegen  ────────►  gen/api/  chi server interface and types
  │                             └─ bff-web implements it
  ├─ Postman import ───────►  api/postman/flowspace.docs.json
  └─ oasdiff in CI  ───────►  a breaking change blocks the merge

api/postman/flowspace.smoke.json   hand-written, small, run by Newman
```

The generated interface is what makes this hold. An operation declared in the
specification and not implemented does not compile. A handler whose types do
not match the specification does not compile. Drift is not detected, it is
prevented.

### Two Postman collections with different jobs

- **Documentation** is regenerated from the specification and never edited by
  hand. Editing it is meaningless, because the next import discards the edit.
- **Smoke** is hand-written, around fifteen requests, and covers health,
  authentication, one create-read-update-delete cycle and a deliberate error.
  It runs against every preview environment, so a removed or renamed endpoint
  breaks it the same day. Its job is to fail when a deployment is broken, not
  to cover the API. Coverage belongs in tests that run in seconds, not in a
  collection that runs against a live environment.

### Versioning

The version is a URL prefix, `/v1/`. Additive changes do not bump it; a
breaking change ships `/v2/` alongside `/v1/`. `oasdiff` runs in CI and blocks
the merge, mirroring what `buf breaking` does for events.

Retiring a version is easier here than it is for events, and the reason is
worth stating: HTTP consumers are online when they call, so access logs say who
is still using `/v1/`. The event log offers no such answer, which is why
ADR-0004 has to be stricter about the same problem.

### Three things declared once, not per endpoint

- **Errors** are RFC 9457 problem details, `application/problem+json`, one
  shape for the entire API. Retrofitting an error shape breaks every client
  that parses errors, so it is decided now rather than discovered later.
- **`Idempotency-Key`** is a required header on every mutation, declared as a
  reusable parameter.
- **Pagination is cursor-based**, never offset. Offset paging over a table with
  concurrent inserts silently skips and repeats rows, and it is meaningless
  under ADR-0008 in any case: pages are filtered by permission after the query
  returns, so an offset does not correspond to a position in anything the
  caller can see.

### What is not in this specification

The internal Connect API is not public and is not described here. Nothing
exposes it to the internet.

The event streams from ADR-0007 are documented in the specification as
`text/event-stream` operations, but their handlers are written by hand on chi
outside the generated interface, because a stream that never ends does not fit
a generated request-response signature. This is a real seam in the decision and
is stated so that nobody discovers it as a surprise.

## Consequences

### Positive

- The compiler links specification and code, so the documented API and the
  running API cannot diverge.
- Documentation is never hand-maintained, so it is never stale.
- A breaking change fails CI rather than a customer's integration.
- One error shape, one idempotency mechanism, one pagination style, declared in
  one place and impossible to forget per endpoint.
- A future SDK or mobile client generates from the same file with no new work.

### Negative / accepted costs

- Adding an endpoint means writing YAML before writing Go. That friction is
  the mechanism, not a side effect, but it is friction on every change.
- Generated handler signatures constrain how handlers are written, and the
  streaming endpoints sit outside them entirely.
- Two Postman collections mean two files where somebody will eventually edit
  the wrong one.
- More generated code in `gen/`, with the merge-conflict cost already accepted
  in ADR-0009.

### Neutral

- Newman keeps exactly the role it is good at, and loses the role it is bad at.

## Alternatives considered

| Option                                                         | Why not                                                                                                                                                                                                     |
| -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The Postman collection as source of truth                      | Hand-maintained with no mechanical link to the routes, so drift is both invisible and permanent; the collection describes the API somebody remembered, not the one that is running                          |
| Code-first generation from annotations                         | The specification becomes an output, so a breaking change can only be detected after it already exists in the code, and comments drift from the handlers they sit above                                     |
| A framework that derives the specification from typed handlers | Genuinely good, and it replaces plain chi handlers with a framework's own request model; rejected to keep the HTTP layer ordinary, and worth revisiting if writing the specification becomes the bottleneck |
| Exposing the internal RPC contract as the public API           | An internal refactor would become a public breaking change, and the two contracts have different audiences and different rates of change                                                                    |
| No specification at all                                        | Documented behaviour becomes whatever the code happens to do, discovered by customers                                                                                                                       |

## Revisit when

- Writing the specification by hand becomes the slowest part of adding an
  endpoint. That is the signal for a framework that derives it from typed
  handlers, not for abandoning specification-first.
- Streaming endpoints outnumber generated ones, at which point the generated
  interface is covering the minority case.
- A second first-party client exists, and generated clients are worth wiring
  into the build rather than writing by hand.

## References

- [OpenAPI Specification](https://spec.openapis.org/oas/latest.html)
- [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
- [RFC 9457 — Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457.html)
- [oasdiff](https://github.com/oasdiff/oasdiff)
