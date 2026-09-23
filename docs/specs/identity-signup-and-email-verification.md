# Spec: Identity signup and email verification

Module id: `identity-signup-and-email-verification`

Status: Draft.

## Objective

Allow a new FlowSpace user to create an account with an email address and password, receive an email code, and prove control of that address. The first clients are API clients using disposable data. A successful signup returns an access token and a refresh token immediately, but the unverified account cannot use Workspace.

This spec defines the account creation and email verification flow. It does not report implementation progress or readiness to serve real users.

## Scope and decision sources

The scope covers signup, the first verification email, requesting a new code, consuming a code, reclaiming an unverified signup after a lost response, and the verified-email state that protected services receive from Identity. Account creation and verification form one capability because signup creates the unverified state and its first verification challenge.

The [specification index](README.md) defines shared project sources. [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md) owns the service boundary, token model, and verified-email rule. [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md) owns the acting subject. Protobuf and generated REST follow [ADR-0005](../adr/0005-one-protobuf-contract-generates-rest.md), [ADR-0007](../adr/0007-version-apis-by-compatibility-boundary.md), and [ADR-0009](../adr/0009-canonical-grpc-errors-map-to-http.md).

Password login after signup, refresh, logout, password reset, provider login and linking, email address changes, and browser token storage are outside this capability. This spec does not choose token lifetimes or signing keys. The later session spec must define those values and refresh behavior without changing the signup outcome agreed here.

## Contract

Use package `flowspace.identity.v1` and an `IdentityService` contract. The proposed public methods use generated REST/JSON under `/v1`. The final Protobuf definitions are the source of truth.

| RPC | Public HTTP route | Request | Successful response |
| --- | --- | --- | --- |
| `CreateAccount` | `POST /v1/accounts` | Email address and password | Stable subject, unverified email state, access token, refresh token, and access-token expiry |
| `RequestEmailVerificationCode` | `POST /v1/email-verification-codes` | Bearer access token | Confirmation that a delivery request was accepted |
| `VerifyEmail` | `POST /v1/email-verifications` | Bearer access token and six-digit code | Verified email state for the authenticated subject |

`CreateAccount` is public and does not accept an acting subject. The other methods require one `Authorization: Bearer <access_token>` header and derive the subject from the validated token. Clients never send a subject or target email to those methods. The code is exactly six ASCII digits; leading zeroes remain significant.

A successful `CreateAccount` response contains an access token and an opaque refresh token from one new device session. The access token follows ADR-0031 and is usable immediately for Identity verification requests. The refresh token is sent only to Identity's future refresh endpoint. The response reports `email_verified=false`. `VerifyEmail` does not need to issue replacement tokens: protected services use Identity's current session check to learn that the email is verified.

The contract uses canonical gRPC errors and the default HTTP mapping. Invalid request shape, malformed email, malformed code, and a password outside the approved policy use `InvalidArgument` (HTTP 400) with safe field details. Missing or invalid authentication uses `Unauthenticated` (HTTP 401). Incorrect, expired, or consumed codes use `InvalidArgument` (HTTP 400) with the same safe message. Rate limits use `ResourceExhausted` (HTTP 429). Internal failures do not expose account data or infrastructure details.

If the email address already belongs to an account, `CreateAccount` returns `AlreadyExists` (HTTP 409) without tokens. If email delivery fails after account creation commits, `CreateAccount` still returns the new session and tokens; Identity keeps the account unverified and retries the queued delivery. An ambiguous or lost signup response does not authorize clients to create another account blindly. A separate email-code claim flow recovers an unverified signup without replaying its original refresh token. Its final RPCs, request fields, responses, and state transition remain open before approval.

## Required behavior

### Account creation

- Identity assigns a stable subject that does not depend on the email address. It stores the account and an unverified email state in its own PostgreSQL database.
- Identity accepts ASCII email addresses and rejects Unicode email addresses. It preserves the local part as entered and compares it case-sensitively. It compares the domain case-insensitively. Implementations must not invent provider-specific rules such as removing dots from an address.
- Identity validates the email and password at the public boundary. It stores a salted, adaptive password hash and never stores or returns the password.
- Identity accepts a password with at least 15 characters without requiring any character type. It also accepts a password with 8–14 characters if it contains at least one lowercase ASCII letter (`a`–`z`) and one ASCII digit (`0`–`9`). It accepts Unicode and counts Unicode code points when measuring length. It rejects passwords shorter than eight characters. These two length paths follow [GitHub's policy](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-strong-password). They depart from [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html#passwordver): MFA is optional for the shorter path, and that path requires a character-type mix.
- Identity applies Unicode NFC normalization before checking password length, checking a blocklist, or hashing. Its maximum allowed password length is at least 64 Unicode code points. It rejects commonly used or compromised passwords and hashes accepted passwords with Argon2id. The starting cost follows [OWASP's minimum](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html): 19 MiB of memory, two iterations, and one degree of parallelism. Real-user use requires a measured cost choice on the deployment host.
- Successful signup creates one device session and returns its access and refresh tokens even though the email is not verified.
- A duplicate signup never returns tokens for the existing account. The owner of an unverified address can start an email-code claim without an access token after losing the original signup response. A successful claim supersedes the pending signup and revokes all of its old sessions. The final contract must define whether the stable subject changes and how the new client receives tokens.
- Signup cannot replay its original response under [ADR-0011](../adr/0011-idempotency-keys-protect-non-idempotent-creates.md), because the response contains a refresh token and Identity stores only its hash. A new ADR must record the claim-flow exception before implementation.

### Email code and verification

- Identity generates a six-digit code with a cryptographically secure random source. It binds the challenge to the account and the current email address, stores a keyed one-way verifier instead of plaintext, and sends the code only to that address.
- A code expires after ten minutes and can succeed once. Five wrong guesses exhaust one challenge. Identity also limits wrong guesses to ten per account per hour and 20 per account per day across challenges and service replicas.
- Requesting a new code invalidates the previous outstanding code for that address. Delivery retries must not create two concurrently valid codes.
- A new code request must be at least 60 seconds after the previous request. Identity issues no more than five codes per account per hour across code request paths and service replicas.
- `VerifyEmail` accepts a valid code for the authenticated account only. It marks the email verified and consumes the code in one transaction. Concurrent use of the same code cannot verify two accounts or succeed twice.
- After verification commits, an Identity session check returns the current verified state for that subject. Workspace denies an unverified subject before workspace authorization and admits a verified subject only if its other access rules pass.
- An expired, guessed, or reused code never changes the verified state. `VerifyEmail` succeeds without changing state if the account is already verified. `RequestEmailVerificationCode` returns `FailedPrecondition` if the account is already verified.

### Abuse and failure handling

- Identity allows at most ten signup requests per source IP per hour. Rate limits also cover code issuance and guesses. The threat model must cover account enumeration, automated signup, email flooding, and online guessing of a six-digit code.
- Error responses and logs never contain passwords, refresh tokens, verification codes, or full verification messages. Code request and email verification responses never contain refresh tokens. The final claim contract must state whether a successful claim issues a new session and refresh token. Security events record outcomes without storing those values.
- Identity commits the account and a durable first-email delivery request together. A delivery failure after the account commit does not change the successful signup response. Identity retries delivery, and the signed-in user can request another code after delivery recovers, subject to request limits. The threat model must cover the confidentiality and removal of queued code material.
- Mailpit captures email in learning environments. Real delivery, MFA, independent backups, and tested restoration remain required before real users join FlowSpace.

## Commands

Run commands from the repository root. These are future implementation checks; this draft does not claim that Identity code or generated contracts already exist.

| Purpose | Command |
| --- | --- |
| Compile Identity binaries without writing output binaries | `go build ./services/identity/cmd/...` |
| Run Identity tests | `go test ./services/identity/...` |
| Run Identity integration tests | `go test -tags=integration ./services/identity/...` |
| Lint source contracts | `task buf -- lint` |
| Generate API code | `task buf -- generate` |
| Check API compatibility against main | `task buf -- breaking --against '.git#branch=main'` |
| Generate query code | `task sqlc -- generate` |

## Testing strategy

Use unit tests for email, password, and code validation, account state transitions, and challenge expiry. Cover ASCII email comparison, both accepted password paths, password blocklist rejection, ten-minute code expiry, and code limits. Cover the password boundaries: seven characters fail, eight characters need a lowercase ASCII letter and an ASCII digit, and 15 characters need no character-type mix. Use PostgreSQL integration tests for unique account creation, one-time code use, concurrent verification, session creation, and durable email delivery.

Test the public generated REST and typed RPC surfaces with API clients. Cover malformed input, unauthorized calls, duplicate signup, lost signup responses, claim codes, rate limits, code resend, stale codes, expired codes, concurrent guesses, and email-delivery failure. Prove that a newly issued token cannot use Workspace before verification and that the same live session passes the verified-email gate after verification. Use Mailpit for end-to-end learning-environment evidence without real users.

## Boundaries

### Always

- Bind verification to the authenticated subject and its current email address.
- Keep unverified accounts out of Workspace even when they hold valid tokens.
- Hash passwords, protect verification codes with a keyed one-way verifier, enforce finite code lifetimes and attempt limits, and use a shared limit store when more than one replica serves requests.
- Keep secrets and full email addresses out of application logs and traces.

### Ask first

- Approve the final claim-flow contract, its exception to ADR-0011, the measured Argon2id cost criterion, and the full spec before this Draft becomes Approved.
- Obtain approval before changing the token issuance outcome or adding an email-change flow.

### Never

- Do not treat possession of an access token as proof of email ownership.
- Do not accept a client-supplied subject or email as authority for verification.
- Do not use an email address as the stable subject or link accounts by matching email addresses.

## Success criteria

Each row describes an observable result. The open contract choices above must be settled before implementation can claim completion.

| Given | Then |
| --- | --- |
| A new user submits a valid email address and password. | Identity creates one unverified account and one session, requests an email code, and returns a stable subject plus access and refresh tokens. |
| A user tries to sign up with an email address that already belongs to an account. | Identity returns `AlreadyExists` (HTTP 409) without tokens. |
| The owner reclaims an unverified signup after losing the signup response. | Identity requires a valid emailed code, supersedes the pending signup, and revokes every old session. |
| The new user calls Workspace with the access token before email verification. | Workspace rejects the request because Identity reports that the email is unverified. |
| The authenticated user submits the valid current code. | Identity consumes the code once and reports that the email is verified. A later session check reports the new state without replacing the tokens. |
| An unverified user submits a malformed, wrong, expired, replaced, or consumed code. | Identity does not verify the email and returns the contract's safe error. |
| The user requests another code. | Only the newest challenge remains valid, subject to the approved request limits. |
| Two requests submit one valid code at the same time. | At most one request consumes the code; the account ends in one verified state. |
| Email delivery fails after account creation. | Signup still returns tokens, the account remains unverified, and durable delivery retries continue. |
| A request exceeds a signup or code limit. | Identity rejects the request without issuing another code or verifying the account. |
| The capability is submitted for implementation review. | Contract, integration, abuse, and workspace-gate tests pass under repository quality checks. |

## Open questions and approval

- Does a successful claim keep the pending account's stable subject or replace it with a new subject, and which sessions and challenges survive?
- What public RPCs, request fields, responses, and error cases define the claim flow? A new ADR must state why signup does not replay the original response under ADR-0011.
- What measured Argon2id cost and performance criterion must pass on the deployment host before real users join?

The owner must approve these answers and the final API contract before this Draft becomes Approved. Approval permits planning; it does not mean that the capability is implemented or ready for real users.
