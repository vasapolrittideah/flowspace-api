# Spec: Identity password recovery

Module id: `identity-password-recovery`

Status: Draft.

## Objective

Allow a FlowSpace account holder who has a verified email address and a password to replace a forgotten password with a six-digit code sent to that address. A successful reset ends every existing session and requires a new password login. The first clients are API clients using disposable data.

This spec defines recovery for an existing password account. It does not report implementation progress or readiness to serve real users.

## Scope and decision sources

The scope covers requesting a recovery code, submitting that code with a new password, revoking sessions, and notifying the account after a password change. These operations share one purpose-bound challenge and one account transition.

The [specification index](README.md) defines shared project sources. [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md) requires six-digit email codes and revocable sessions. The [signup and verification spec](identity-signup-and-email-verification.md) owns email comparison, password policy, code protection, and shared code limits. The [password login and sessions spec](identity-password-login-and-sessions.md) owns session checks and token lifetimes. The [Identity threat model](../security/identity-threat-model.md) names the recovery threats. The [OWASP forgot-password guidance](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html) informs the generic request response, post-reset notice, and fresh-login behavior. Protobuf and generated REST follow [ADR-0005](../adr/0005-one-protobuf-contract-generates-rest.md), [ADR-0007](../adr/0007-version-apis-by-compatibility-boundary.md), and [ADR-0009](../adr/0009-canonical-grpc-errors-map-to-http.md).

An unverified account whose email owner does not know its password uses the account-claim flow in the signup spec. A provider-only account cannot acquire a password through anonymous recovery. Provider login and linking, email changes, authenticated password changes, MFA recovery, and browser token storage are outside this capability.

## Contract

Use package `flowspace.identity.v1` and the `IdentityService` contract. Public RPCs expose generated REST/JSON under `/v1`. Source Protobuf definitions will fix the final field and method names.

| RPC | Public HTTP route | Request | Successful response |
| --- | --- | --- | --- |
| `RequestPasswordResetCode` | `POST /v1/password-reset-codes` | `email`, no bearer token | `accepted=true`, whether or not an eligible account exists |
| `ResetPassword` | `POST /v1/password-resets` | `email`, `code`, and `new_password`, no bearer token | Confirmation that the password changed and sessions were revoked; no tokens |

Both methods use the email comparison rules from the signup spec. The code is exactly six ASCII digits as a string, including leading zeroes. Clients never supply a subject, session ID, or verified-email state. A reset response never includes an access token, refresh token, password, or code. Successful reset does not create a session; the client calls `CreatePasswordSession` with the new password.

`RequestPasswordResetCode` returns the same `accepted=true` response for a missing account, an unverified account, a provider-only account, and an eligible account. It sends email only for an eligible account. Responses and practical timing must not reveal eligibility. A malformed email returns `InvalidArgument` (HTTP 400) without revealing account state.

`ResetPassword` uses `InvalidArgument` (HTTP 400) with one safe recovery-code error for a wrong, expired, replaced, consumed, or ineligible code. Invalid password shape or policy uses safe field details. A request limit uses `ResourceExhausted` (HTTP 429). An unavailable database or durable delivery store uses `Unavailable` (HTTP 503). No error exposes account existence, session state, credentials, or internal diagnostics. The gateway uses the default canonical gRPC-to-HTTP mapping.

## Required behavior

### Requesting a code

- Identity accepts a reset request only for an active account with a verified current email address and a password credential. A linked provider does not remove eligibility if the account also has a password. A provider-only or unverified account receives the same public result but no reset code.
- Identity generates a six-digit code with a cryptographically secure random source. It binds the challenge to the account, current email, and password-reset purpose. The challenge record stores a keyed one-way verifier, not the code. Verification and claim codes cannot reset a password, and a reset code cannot verify or claim an account.
- Identity commits the challenge and a durable email-delivery request together. A later delivery failure leaves the challenge pending and retries delivery. Queued code material is encrypted at rest, access-limited, and removed after delivery or expiry. Delivery retries cannot create another valid challenge.
- A new reset-code request replaces the prior reset challenge only. It does not invalidate verification or claim challenges. A delayed email for an older challenge cannot make that code valid again.
- The challenge expires ten minutes after issue and can succeed once. Identity issues a new code only when at least 60 seconds have passed since the previous request. It issues no more than five codes per account per hour across verification, claim, and reset requests. Source-address and account limits use shared state across replicas. The same public limit behavior applies to missing and ineligible accounts.
- Code requests do not change the password, verified-email state, provider links, or sessions. Identity never locks an account or revokes a session because someone requested recovery.

### Completing a reset

- `ResetPassword` requires the current code and a new password that meets the signup spec's normalization, length, blocklist, and Argon2id rules. Identity validates request shape and limits before expensive password work.
- Five wrong guesses exhaust one challenge. Identity also allows at most ten wrong guesses per account per hour and 20 per account per day across verification, claim, and reset challenges. The limits apply across replicas. The same safe code error covers an unknown or ineligible account and every unusable code.
- A valid reset atomically consumes the code, replaces the password hash, invalidates other outstanding reset challenges, revokes every existing session and refresh token for the subject, and records a durable password-change notice. The stable subject, verified-email state, and provider links do not change.
- Identity serializes reset with password login, session creation, and refresh for the same subject. A login using the old password cannot commit a new session after the reset commits. A session check that starts after that commit rejects every earlier session, even if its access token has not expired. A request admitted before the commit can finish.
- Identity reports success only after the password and revocation transaction commits. It does not issue tokens or sign the caller in. The new password works through the ordinary password-login method; the old password and all old refresh tokens fail.
- Identity sends a notice to the verified email address after a successful reset. The notice contains no password or recovery code. If delivery fails after the reset commits, Identity retries the durable notice without restoring any session or code.
- If a reset response is lost, a retry with the consumed code receives the safe code error. The user can test the outcome by logging in with the new password. If the reset did not commit, the current code remains usable until it expires or is exhausted.

### Abuse and failure handling

- Public request and guess limits apply by trusted source address and an account-independent keyed email identifier. Missing accounts consume limit state too. Only configured edge proxies can supply the source address. If shared limit state is unavailable, Identity returns a temporary error rather than bypassing the limit. An unknown or ineligible account follows comparable code-verifier work so invalid-code timing does not reveal eligibility.
- Errors, logs, traces, and metrics never contain full email addresses, passwords, codes, access tokens, refresh tokens, or email bodies. Security events record code-request, invalid-code, reset, session-revocation, and delivery outcomes without reusable secrets.
- Identity applies the ordinary five-second request cap and shorter caller deadlines under [ADR-0010](../adr/0010-cap-ordinary-unary-requests-at-five-seconds.md). Cancellation or a lost response does not undo a committed reset. If the durable delivery request cannot commit, the password and sessions remain unchanged.
- Mailpit captures recovery email in learning environments. Real email delivery, MFA, independent backups, and tested restoration remain required before real users join FlowSpace.

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

Unit tests cover email comparison, password policy, purpose binding, code format, ten-minute expiry, and the exact request and guess limits. Use a controlled clock for expiry tests. PostgreSQL integration tests cover challenge replacement, single use, concurrent guesses and resets, atomic password and session changes, a racing old-password login, and notice delivery after commit.

Public REST and typed RPC tests cover a missing, unverified, verified password, linked-provider password, and provider-only account. Compare generic request responses, practical timing, rate-limit behavior, and invalid-code errors for eligible and ineligible accounts. Exercise malformed input, wrong-purpose codes, an older email arriving late, failed delivery, cancellation, and a lost reset response. Mailpit and cross-service tests prove that reset sends the code and notice, old sessions fail immediately after commit, and login with the new password creates a fresh session.

The abuse tests cover ID-T01, ID-T03, ID-T04, ID-T05, ID-T12, ID-T16, ID-T17, and ID-T19 in the [threat model](../security/identity-threat-model.md). Recovery data retention and restoration tests also cover ID-T21.

## Boundaries

### Always

- Bind recovery to the verified current email and the existing password account without changing its subject.
- Use single-use purpose-bound codes, shared request and guess limits, and one transaction for the password change and all-session revocation.
- Require a new login after reset and queue a password-change notice without exposing secrets.

### Ask first

- Obtain approval before changing the all-session revocation rule, code limits, eligible account types, or new-login requirement.
- Approve numeric source-address limits and the handling of anonymous code-request retries before implementation.

### Never

- Do not create a password for a provider-only account from a matching email address.
- Do not treat an unverified email, an access token, or a client-supplied subject as proof for anonymous recovery.
- Do not leave an old session active, issue tokens from reset, or report success before revocation commits.

## Success criteria

Each row describes an observable result required before implementation can claim completion. The last column links its abuse case to the threat model.

| Given | Then | Threat |
| --- | --- | --- |
| An eligible account requests a reset code. | Identity queues one six-digit email code and returns `accepted=true` without changing the password or sessions. | ID-T01, ID-T03, ID-T04 |
| A missing, unverified, or provider-only account requests a code. | Identity returns the same accepted response and practical timing without sending a code or revealing eligibility through limits. | ID-T01, ID-T05 |
| A user requests another code after the allowed interval. | Only the new reset code works; verification and claim codes remain independent. | ID-T03, ID-T04 |
| A valid reset code and approved new password arrive. | Identity changes the password once, consumes all reset challenges, revokes all sessions, queues a notice, and returns no tokens. | ID-T04, ID-T05, ID-T12 |
| A caller supplies a malformed, wrong, expired, replaced, consumed, or wrong-purpose code. | No password or session changes, and all unusable codes return the safe recovery error. | ID-T03, ID-T04, ID-T05 |
| A caller exhausts request or guess limits, including across replicas or missing accounts. | Identity denies further attempts without disclosing account eligibility or bypassing an unavailable limit store. | ID-T01, ID-T03, ID-T16 |
| Two clients submit one valid code or an old-password login races with reset. | Only one reset commits, and no session authenticated with the old password commits after that reset. | ID-T05, ID-T12 |
| A reset commits while an old access token has time remaining. | The next live session check rejects it and every old refresh token; a request admitted before commit can finish. | ID-T05, ID-T09, ID-T13 |
| A successful reset response is lost. | Reusing the code cannot reset again; the new password can establish a fresh session without replaying old tokens. | ID-T04, ID-T05 |
| Email delivery fails before or after a required transaction. | A failed durable queue commit leaves credentials and sessions unchanged; a failure after commit retries delivery without reversing the reset. | ID-T05, ID-T16 |
| The capability is submitted for implementation review. | Contract, abuse, concurrency, delivery, telemetry, and cross-service tests pass under repository quality checks. | ID-T17, ID-T19, ID-T21 |

## Open questions and approval

The draft needs approval for recovery eligibility, numeric source-address request and guess limits, and the retry contract of the anonymous code-request method. Those choices must preserve generic responses for eligible and ineligible accounts. The all-session revocation and new-login behavior were confirmed for this draft.
