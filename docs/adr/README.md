# Architecture Decision Records

One record documents one architectural decision that would be expensive to reverse. The [architecture](../architecture.md) describes the accepted system direction; a record here explains why one part of that direction was chosen and what was rejected instead. Replaceable tools and libraries remain in the [technology stack](../technology-stack.md).

These records were written on 2026-09-14 from the accepted rationale in the [architecture](../architecture.md). The date on each record is the recording date, not an implementation or production-readiness date.

An accepted record is never edited or deleted. When a decision changes, add a new record that identifies the record it supersedes and update the old record's status.

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-one-bounded-context-per-service.md) | One bounded context per independently deployable service | Accepted; service count and Keycloak boundary superseded by [0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) |
| [0002](0002-one-repository-and-one-go-module.md) | One repository and one root Go module | Accepted |
| [0003](0003-hexagonal-layers-inside-each-service.md) | Hexagonal layers inside each service | Accepted |
| [0004](0004-share-only-technical-packages-across-services.md) | Only technical packages are shared across services | Accepted |
| [0005](0005-one-protobuf-contract-generates-rest.md) | One Protobuf contract generates the REST surface | Accepted |
| [0006](0006-version-events-separately-from-rpc.md) | Event contracts are versioned separately from RPC contracts | Accepted |
| [0007](0007-version-apis-by-compatibility-boundary.md) | APIs are versioned by compatibility boundary | Accepted |
| [0008](0008-list-methods-use-cursor-pagination.md) | List methods use bounded cursor pagination | Accepted |
| [0009](0009-canonical-grpc-errors-map-to-http.md) | Canonical gRPC errors map to HTTP | Accepted |
| [0010](0010-cap-ordinary-unary-requests-at-five-seconds.md) | Ordinary unary requests are capped at five seconds | Accepted |
| [0011](0011-idempotency-keys-protect-non-idempotent-creates.md) | Idempotency keys protect non-idempotent creates | Accepted |
| [0012](0012-field-masks-and-action-rpcs-model-updates.md) | Field masks and action RPCs model updates | Accepted |
| [0013](0013-acting-identity-comes-from-the-token.md) | Acting identity comes from the validated token | Accepted |
| [0014](0014-relational-data-uses-explicit-sql.md) | Relational data uses explicit SQL | Accepted |
| [0015](0015-each-service-owns-a-postgresql-instance.md) | Each service owns a PostgreSQL instance | Accepted |
| [0016](0016-cross-service-side-effects-use-domain-events.md) | Cross-service side effects use domain events | Accepted |
| [0017](0017-outbox-and-idempotent-consumers-deliver-events.md) | An outbox and idempotent consumers deliver events | Accepted |
| [0018](0018-task-mutations-reject-stale-versions.md) | Task mutations reject stale versions | Accepted |
| [0019](0019-keycloak-owns-authentication-flows.md) | Keycloak owns authentication flows | Superseded by [0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) |
| [0020](0020-services-authorize-from-bounded-local-projections.md) | Services authorize from bounded local projections | Accepted |
| [0021](0021-run-kubernetes-locally-and-on-the-self-hosted-server.md) | Kubernetes runs locally and on the self-hosted server | Accepted |
| [0022](0022-single-host-storage-holds-disposable-data.md) | Single-host storage holds disposable learning data | Accepted |
| [0023](0023-hosted-ci-stays-within-the-zero-spend-limit.md) | Hosted CI stays within the zero-spend limit | Accepted |
| [0024](0024-gitops-deploys-versioned-images.md) | GitOps deploys versioned images | Accepted |
| [0025](0025-environments-overlay-shared-manifests.md) | Environments overlay shared manifests | Accepted |
| [0026](0026-an-outbound-tunnel-exposes-selected-routes.md) | An outbound tunnel exposes selected routes | Accepted |
| [0027](0027-git-contains-only-encrypted-kubernetes-secrets.md) | Git contains only encrypted Kubernetes secrets | Accepted |
| [0028](0028-telemetry-is-vendor-neutral-and-correlated.md) | Telemetry is vendor-neutral and correlated | Accepted |
| [0029](0029-the-observability-stack-is-self-hosted.md) | The observability stack is self-hosted | Accepted |
| [0030](0030-telemetry-is-bounded-and-non-blocking.md) | Telemetry is bounded and non-blocking | Accepted |
| [0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) | FlowSpace owns authentication and revocable sessions | Accepted |
| [0032](0032-identity-signup-recovers-with-login.md) | Identity signup recovers with login | Accepted; scoped exception to [0011](0011-idempotency-keys-protect-non-idempotent-creates.md) |
| [0033](0033-identity-token-issuance-does-not-replay-responses.md) | Identity token issuance does not replay responses | Accepted; scoped exception to [0011](0011-idempotency-keys-protect-non-idempotent-creates.md) |
| [0034](0034-provider-login-uses-a-one-time-handoff.md) | Provider login uses a one-time handoff | Accepted; scoped exceptions to [0005](0005-one-protobuf-contract-generates-rest.md) and [0011](0011-idempotency-keys-protect-non-idempotent-creates.md) |
| [0035](0035-identity-signup-security-and-mail-delivery.md) | Identity signup security and mail delivery | Proposed |
