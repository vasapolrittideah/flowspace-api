# Spec: Identity provider login

Module id: `identity-provider-login`

Status: Approved.

## Objective

Allow a FlowSpace user to sign in with Google or GitHub through an API client. Identity creates a FlowSpace session only after it checks the provider response. A first-time user can receive a new provider-only account when no FlowSpace account owns the verified email address.

The first release uses disposable data. This spec does not claim implementation progress or readiness for real users.

## Scope and decision sources

This capability covers provider authorization, callback validation, API-client handoff, account lookup or creation, and session issuance. Google and GitHub are the only providers. A separate capability will define how an authenticated user links or unlinks a provider. Email changes, browser token storage, MFA, and other providers are outside this spec.

[ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md) owns provider identity, email verification, and FlowSpace sessions. [ADR-0034](../adr/0034-provider-login-uses-a-one-time-handoff.md) owns the callback adapter and retry rules. The [signup spec](identity-signup-and-email-verification.md) owns email comparison, account uniqueness, and six-digit verification codes. The [password login and sessions spec](identity-password-login-and-sessions.md) owns token format, lifetimes, refresh, and logout. The [Identity threat model](../security/identity-threat-model.md) defines the abuse cases below. The [OAuth security guidance](https://www.rfc-editor.org/rfc/rfc9700.html), [Google OpenID Connect guide](https://developers.google.com/identity/openid-connect/openid-connect), and [GitHub OAuth guide](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps) define the provider protocol requirements.

## Contract

Use package `flowspace.identity.v1` and the `IdentityService` contract. Public RPCs expose generated REST/JSON under `/v1`. Source Protobuf definitions will fix final field and method names.

| RPC or callback | Public HTTP route | Request | Successful response |
| --- | --- | --- | --- |
| `StartProviderLogin` | `POST /v1/provider-login-attempts` | `provider` set to `GOOGLE` or `GITHUB`; no bearer token | Provider authorization URL, opaque `attempt_token`, and expiry |
| Provider callback | `GET /v1/provider-login-callbacks/{provider}` | Provider `code` and `state`, or provider error | Browser page with a one-time handoff code after successful validation; no FlowSpace tokens |
| `CreateProviderSession` | `POST /v1/provider-sessions` | `attempt_token` and handoff code; no bearer token | Stable subject, current `email_verified`, access token, refresh token, and expiry times |

The callback is a thin provider-facing HTTP adapter. It performs no account or session rule itself. The API client opens the authorization URL in a browser, copies the handoff code from the callback page, and calls `CreateProviderSession`. The attempt token stays in the API client. Neither the provider URL nor the callback page contains a FlowSpace access or refresh token.

The callback and handoff code expire with the login attempt after ten minutes. The attempt token and handoff code each have at least 128 random bits. Identity stores only keyed verifiers of both values. Each code and attempt succeeds once. A browser page cannot claim a session without the attempt token, and an API client cannot claim one without the handoff code.

`CreateProviderSession` returns the same token and expiry fields as `CreatePasswordSession`. It does not return provider tokens. A pending, expired, consumed, or failed attempt returns a safe error without tokens. A new provider identity with no usable verified email and one whose email belongs to another FlowSpace account receive the same `FailedPrecondition` error, HTTP 400 status, response shape, message, and practical timing. The message is `Unable to complete provider login. Check the provider email or sign in through another method to link the provider.` A provider outage returns `Unavailable` (HTTP 503). Malformed input returns `InvalidArgument` (HTTP 400). Limits return `ResourceExhausted` (HTTP 429). Identity uses the canonical gRPC-to-HTTP mapping and does not expose provider diagnostics to clients.

`StartProviderLogin` and `CreateProviderSession` reject the `Idempotency-Key` header. Neither method promises to replay a successful response. [ADR-0034](../adr/0034-provider-login-uses-a-one-time-handoff.md) records this exception to ADR-0011.

## Required behavior

### Starting and completing authorization

- Identity allows only configured Google and GitHub endpoints and exact registered callback URLs. Clients cannot supply provider URLs, callback URLs, or post-login redirect targets.
- Identity uses the authorization code flow with PKCE `S256` and a fresh one-time `state` value for both providers. It binds the state, PKCE verifier, provider, and attempt token to one login attempt. Google also receives a fresh `nonce` that Identity checks in its ID token.
- Identity sends the user to the selected provider. The provider returns an authorization code to its fixed Identity callback. Identity rejects a missing, changed, expired, or reused state before exchanging the code. It rejects a callback delivered to the wrong provider route.
- Identity exchanges the code server-side with the matching provider and the original PKCE verifier. It checks the redirect URI used for that attempt. Provider tokens stay on the server and are discarded after Identity obtains the identity evidence.
- The callback records one validated provider result and creates a one-time handoff code. It creates no FlowSpace account or session. A provider denial or failed exchange records failure and yields no handoff code. Repeated callbacks cannot replace a validated result.
- The callback page has no third-party resources. It sets `Cache-Control: no-store` and `Referrer-Policy: no-referrer`. Identity does not put a provider code, provider token, attempt token, or handoff code in a redirect URL or telemetry.

### Provider identity and email

- For Google, Identity checks the ID token signature against Google's published keys. It checks issuer, audience, expiry, nonce, and nonempty `sub`. It uses `sub` as the provider identity, never the email address. A missing or false `email_verified` value provides no verified-email proof.
- For GitHub, Identity calls the authenticated user endpoint after exchanging the code. It uses the stable numeric user `id` as the provider identity. It requests only the scope needed to read email addresses. When the provider subject is new, Identity checks the primary verified email through GitHub's authenticated email endpoint. A profile email alone is not verified-email proof.
- Before creating a new account, Identity validates the provider email using the signup spec's comparison rules. It uses verified-email evidence only after it checks the provider response. It never trusts client-supplied email, provider subject, or verified state.
- A known pair of provider name and provider subject identifies its existing FlowSpace account even when the provider email changes, loses verification, or is absent. Identity does not change the stored FlowSpace email or its verified state during provider login. The existing account keeps its Workspace access when its stored email is verified. Identity still requires valid proof of the provider subject. A GitHub email-endpoint failure does not block login for an existing link once GitHub's authenticated user endpoint proves its subject.
- A new provider identity needs a usable verified email from the provider. Google must supply an email with `email_verified=true`. GitHub must supply a primary email with `verified=true` through the authenticated email endpoint. If this proof or the email is missing, Identity creates no account, link, or session. The first release does not ask the user to supply another email during provider login.
- If the new provider identity has an unused usable email, Identity creates one provider-only account. It records a new stable subject, the email, and a unique provider link in one transaction. The account has no password credential. A verified Google `@gmail.com` address or Google Workspace address with an `hd` claim starts with `email_verified=true` after Identity checks the ID token. Other Google addresses and GitHub addresses start with `email_verified=false` even when the provider marks them verified. [Google's ID token guide](https://developers.google.com/identity/gsi/web/guides/verify-google-id-token) explains why a verified third-party Google email does not prove current mailbox control.
- For a new account that starts unverified, Identity creates a six-digit email challenge and a durable delivery request in the account transaction. The user receives a FlowSpace session but cannot use Workspace until `VerifyEmail` succeeds. The challenge uses the shared lifetime, guess, resend, delivery, and source limits from the signup spec. If delivery fails after the transaction commits, Identity retries delivery and the account remains unverified.
- If another FlowSpace account owns the email, Identity creates no account, link, or session. The user must sign in through an existing method and use the separate provider-link flow. Matching email addresses never link accounts automatically.

### Session, failure, and abuse handling

- `CreateProviderSession` consumes the handoff proof and creates one session in one transaction. A concurrent attempt can create at most one account, link, and session. It applies the ten-minute access-token, 30-day idle-session, and 90-day absolute-session limits from the sessions spec.
- An unverified FlowSpace account cannot use Workspace even if it receives a session. Identity returns the current account verification state through live session checks. Workspace still owns membership and roles.
- If a `StartProviderLogin` response is lost, a retry creates a new attempt and the old attempt expires after ten minutes. If a successful `CreateProviderSession` response is lost, the user starts a new provider login. The first session can remain active until it expires or the user logs out. Identity never stores a refresh token in a form that it can replay.
- Identity allows at most 60 `StartProviderLogin` requests and 120 provider callbacks per trusted source IP per hour. Callback limits count successful and invalid requests. Five failed handoff claims exhaust one attempt. Identity also allows at most 100 failed `CreateProviderSession` requests per source IP per hour, including unknown attempt tokens. Limits count retries, use shared state across replicas, and fail closed when that state is unavailable. Only configured edge proxies can supply the source address.
- Only the callback page displays the one-time handoff code. Logs, traces, metrics, errors, and all other browser content omit provider codes and tokens, FlowSpace tokens, handoff secrets, and full email addresses. Security events record provider, operation, and outcome without reusable secrets.
- A timeout on a required provider call, denied consent, invalid proof, or database failure creates no partial account, link, or session. Identity reports success only after the account, link, and session transaction commits.

## Commands

Run commands from the repository root during implementation. Approval does not mean that Identity code or contracts exist.

| Purpose | Command |
| --- | --- |
| Compile Identity binaries | `go build ./services/identity/cmd/...` |
| Run Identity tests | `go test ./services/identity/...` |
| Run Identity integration tests | `go test -tags=integration ./services/identity/...` |
| Lint source contracts | `task buf -- lint` |
| Generate API code | `task buf -- generate` |
| Check API compatibility against main | `task buf -- breaking --against '.git#branch=main'` |
| Generate query code | `task sqlc -- generate` |

## Testing strategy

Unit tests cover attempt expiry, single use, state and nonce checks, provider selection, email comparison, and safe errors. Provider-adapter tests use safe fixtures for altered Google claims and GitHub user or email responses. They reject wrong issuer, audience, nonce, subject, signature, PKCE verifier, provider, callback URL, and email proof for a new provider identity. Tests prove that a known provider subject can log in when its email changes or is absent.

Public REST and callback tests cover a successful login for each provider, first account creation, returning login, missing provider email, email collision, denied consent, lost responses, and expired or repeated handoff proof. They compare the email-collision and unusable-email results, including status, body, and practical timing. They cover the 60-start, 120-callback, five-failure-per-attempt, and 100-failure-per-IP limits across replicas. Concurrency tests prove one account, link, and session transition. Email tests prove that Google Gmail and Workspace accounts start verified, while other Google and GitHub accounts require a FlowSpace code before Workspace access. Capture the callback page and telemetry to prove that no reusable secret appears. Run live provider smoke tests only with disposable accounts.

Callback CSRF, code injection, provider mix-up, PKCE, and redirects test ID-T06. Provider proof, email verification, and account-collision tests cover ID-T01, ID-T03, ID-T07, and ID-T08. Handoff replay, account races, and session issuance cover ID-T09, ID-T11, and ID-T12. Input, outage, rate-limit, secret-leak, and expiry tests cover ID-T15, ID-T16, ID-T17, and ID-T19. Dependency review and data retention cover ID-T20 and ID-T21 in the [threat model](../security/identity-threat-model.md).

## Boundaries

### Always

- Bind each provider result to one short-lived attempt and require separate browser and API-client proof before issuing FlowSpace tokens.
- Identify a provider account by provider name and stable provider subject. Use the shared FlowSpace session rules after login.
- Keep provider tokens server-side, verify email evidence, and preserve the verified-email gate before Workspace access.

### Ask first

- Obtain approval before changing the one-time API-client handoff or placing FlowSpace tokens on a browser page.
- Obtain approval before changing provider email rules, account-collision responses, or automatic linking rules.
- Obtain approval before changing the approved attempt, callback, or handoff limits.

### Never

- Do not link accounts by matching email addresses or move a provider link between subjects during login.
- Do not accept provider profile data, a client-supplied provider token, or an unverified email as identity proof.
- Do not put a reusable secret in a URL, browser page, log, trace, metric, or shared cache.

## Success criteria

Each row describes an observable result required before implementation can claim completion. The last column links its abuse case to the threat model.

| Given | Then | Threat |
| --- | --- | --- |
| An API client starts Google or GitHub login. | Identity returns one provider authorization URL and a separate secret attempt token. The URL cannot select an unapproved provider or redirect target. | ID-T06, ID-T15 |
| A provider calls back with a valid code and state. | Identity checks provider proof once and displays a one-time handoff code without FlowSpace tokens. | ID-T06, ID-T07, ID-T17 |
| A callback has a wrong provider, state, nonce, verifier, redirect URI, issuer, or audience. | Identity rejects it without issuing a handoff code, account, link, or session. | ID-T06, ID-T07 |
| A first-time provider identity has an unused verified email. | Identity creates one provider-only account, one link, and one session for a new stable subject. | ID-T07, ID-T08, ID-T12 |
| A new Google identity has a verified Gmail or Google Workspace email. | The new account starts verified and can pass Workspace's verified-email gate. | ID-T07, ID-T14 |
| A new Google identity has a verified third-party email, or a new GitHub identity has a verified primary email. | The new account receives a session and a six-digit email challenge; Workspace rejects it until FlowSpace verifies the email. | ID-T03, ID-T07, ID-T14 |
| A new provider-only account completes its FlowSpace email challenge. | The next live session check reports `email_verified=true`, so Workspace can apply its membership rules. | ID-T03, ID-T14 |
| A provider identity already has a FlowSpace link, but its provider email changed or is missing. | Identity signs in the same subject, preserves the stored FlowSpace email and verification state, and does not change Workspace access. | ID-T07, ID-T08 |
| A verified provider email already belongs to another FlowSpace account, or a new provider identity lacks a usable verified email. | Identity creates no new account, link, or session and returns the same safe status and response. | ID-T01, ID-T08 |
| A client supplies an invalid, expired, or used handoff proof. | No session is issued, and no reusable secret appears in the error. | ID-T09, ID-T12, ID-T17 |
| Two clients claim the same handoff proof concurrently. | At most one account, link, and session transition commits. | ID-T12 |
| A client loses a successful `StartProviderLogin` response. | A retry creates a new attempt. The old attempt expires without an account or session. | ID-T12, ID-T19 |
| A client loses a successful `CreateProviderSession` response. | A new provider login issues a new token pair for the same subject without replaying the lost refresh token. The first session can remain active. | ID-T11, ID-T12 |
| A required provider call or Identity dependency fails after authorization starts. | Identity leaves no partial account, link, or session and reports a safe temporary failure. | ID-T07, ID-T16 |
| A request exceeds an approved provider-login limit. | Identity rejects it without issuing another attempt, handoff code, or session. | ID-T06, ID-T16 |
| The capability is submitted for implementation review. | Contract, provider-adapter, abuse, concurrency, telemetry, and cross-service tests pass under repository quality checks. | ID-T15, ID-T17, ID-T20 |

## Before real users join

The first release uses disposable accounts. Before real users join, FlowSpace needs real email delivery, tested restoration, and the account protection required by [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md). A separate decision must define how a provider-only user recovers access after losing the provider account. Test the approved IP limits with users behind shared networks before treating those limits as ready for real traffic.
