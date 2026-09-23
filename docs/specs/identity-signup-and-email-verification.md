# Spec: Identity signup and email verification

Module id: `identity-signup-and-email-verification`

Status: Approved.

## Objective

Allow a new FlowSpace user to create an account with an email address and password, receive an email code, and prove control of that address. The first clients are API clients using disposable data. A successful signup returns an access token and a refresh token immediately, but the unverified account cannot use Workspace.

This spec defines the account creation and email verification flow. It does not report implementation progress or readiness to serve real users.

## Scope and decision sources

The scope covers signup, the first verification email, requesting a new code, consuming a code, reclaiming an unverified signup when the email owner does not know its password, and the verified-email state that protected services receive from Identity. Account creation and verification form one capability because signup creates the unverified state and its first verification challenge.

The [specification index](README.md) defines shared project sources. [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md) owns the service boundary, token model, and verified-email rule. [ADR-0013](../adr/0013-acting-identity-comes-from-the-token.md) owns the acting subject. [ADR-0032](../adr/0032-identity-signup-recovers-with-login.md) explains the signup retry exception to ADR-0011. Protobuf and generated REST follow [ADR-0005](../adr/0005-one-protobuf-contract-generates-rest.md), [ADR-0007](../adr/0007-version-apis-by-compatibility-boundary.md), and [ADR-0009](../adr/0009-canonical-grpc-errors-map-to-http.md).

Password login after signup, refresh, logout, password reset, provider login and linking, email address changes, and browser token storage are outside this capability. A user who loses the signup response will recover through password login once that capability exists. Until then, this flow uses disposable data. This spec does not choose token lifetimes or signing keys. The later session spec must define those values and refresh behavior without changing the signup outcome agreed here.

## Contract

Use package `flowspace.identity.v1` and an `IdentityService` contract. The approved public methods use generated REST/JSON under `/v1`. The final Protobuf definitions are the source of truth.

| RPC | Public HTTP route | Request | Successful response |
| --- | --- | --- | --- |
| `CreateAccount` | `POST /v1/accounts` | Email address and password | Stable subject, unverified email state, access token, refresh token, and access-token expiry |
| `RequestEmailVerificationCode` | `POST /v1/email-verification-codes` | Bearer access token | Confirmation that a delivery request was accepted |
| `VerifyEmail` | `POST /v1/email-verifications` | Bearer access token and six-digit code | Verified email state for the authenticated subject |
| `RequestUnverifiedAccountClaimCode` | `POST /v1/unverified-account-claim-codes` | Email address, no bearer token | Confirmation that the request was accepted, regardless of account state |
| `ClaimUnverifiedAccount` | `POST /v1/unverified-account-claims` | Email address, claim code, and new password, no bearer token | New subject, verified email state, new access token, new refresh token, and access-token expiry |

`CreateAccount` and both claim methods are public and do not accept an acting subject. `RequestEmailVerificationCode` and `VerifyEmail` require one `Authorization: Bearer <access_token>` header and derive the subject from the validated token. Clients never send a subject or target email to those authenticated methods. Verification and claim codes are exactly six ASCII digits; leading zeroes remain significant.

The request fields are `email` and `password` for `CreateAccount`, no body fields for `RequestEmailVerificationCode`, `code` for `VerifyEmail`, `email` for `RequestUnverifiedAccountClaimCode`, and `email`, `code`, and `new_password` for `ClaimUnverifiedAccount`. The two code-request methods return `accepted=true`. `VerifyEmail` returns `email_verified=true`. The successful claim response uses the same token and expiry fields as `CreateAccount`, with `email_verified=true` and the new subject. Generated REST/JSON follows Protobuf JSON field naming.

A successful `CreateAccount` response contains an access token and an opaque refresh token from one new device session. The access token follows ADR-0031 and is usable immediately for Identity verification requests. The refresh token is sent only to Identity's future refresh endpoint. The response reports `email_verified=false`. `VerifyEmail` does not need to issue replacement tokens: protected services use Identity's current session check to learn that the email is verified.

The contract uses canonical gRPC errors and the default HTTP mapping. Invalid request shape, malformed email, malformed code, and a password outside the approved policy use `InvalidArgument` (HTTP 400) with safe field details. Missing or invalid authentication uses `Unauthenticated` (HTTP 401). Incorrect, expired, or consumed codes use `InvalidArgument` (HTTP 400) with the same safe message for each code purpose. An ineligible claim uses that same claim error. Rate limits use `ResourceExhausted` (HTTP 429). Internal failures do not expose account data or infrastructure details.

If the email address already belongs to an account, `CreateAccount` returns `AlreadyExists` (HTTP 409) without tokens. If email delivery fails after account creation commits, `CreateAccount` still returns the new session and tokens; Identity keeps the account unverified and retries the queued delivery. `CreateAccount` and `ClaimUnverifiedAccount` reject `Idempotency-Key` rather than promise response replay. If a signup response is lost, a retry either creates the account if the first attempt did not commit or returns `AlreadyExists` if it did. A user who knows the signup password then logs in to obtain new tokens. This capability does not claim to recover a lost response before password login exists.

## Required behavior

### Account creation

- Identity assigns a stable subject that does not depend on the email address. It stores the account and an unverified email state in its own PostgreSQL database.
- Identity accepts ASCII email addresses and rejects Unicode email addresses. It preserves the local part as entered and compares it case-sensitively. It compares the domain case-insensitively. Implementations must not invent provider-specific rules such as removing dots from an address.
- Identity validates the email and password at the public boundary. It stores a salted, adaptive password hash and never stores or returns the password.
- Identity accepts a password with at least 15 characters without requiring any character type. It also accepts a password with 8–14 characters if it contains at least one lowercase ASCII letter (`a`–`z`) and one ASCII digit (`0`–`9`). It accepts Unicode and counts Unicode code points when measuring length. It rejects passwords shorter than eight characters. These two length paths follow [GitHub's policy](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-strong-password). They depart from [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html#passwordver): MFA is optional for the shorter path, and that path requires a character-type mix.
- Identity applies Unicode NFC normalization before checking password length, checking a blocklist, or hashing. Its maximum allowed password length is at least 64 Unicode code points. It rejects commonly used or compromised passwords and hashes accepted passwords with Argon2id. The first release uses `m=19456` KiB (19 MiB), `t=2`, and `p=1`, which matches [OWASP's minimum](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html). The deployment-host benchmark validates this fixed choice before real users join; it does not select new parameters.
- Successful signup creates one device session and returns its access and refresh tokens even though the email is not verified.
- A duplicate signup never returns tokens for the existing account. A user who loses the signup response but knows the password recovers through password login when that capability exists. Until then, API clients use disposable addresses for this failure case.
- `CreateAccount` does not accept `Idempotency-Key`. It cannot replay its original refresh token because Identity stores only a hash. [ADR-0032](../adr/0032-identity-signup-recovers-with-login.md) records this scoped exception to ADR-0011.

### Claiming an unverified account

- An email owner who does not know the password of a pending unverified signup can request a new claim code without an access token. Identity sends a code only if an unverified account exists. It returns the same `accepted=true` response when no account exists or the account is already verified. Public request limits and observable response timing must not disclose whether the account exists or is eligible.
- A claim code is bound to the email address, pending account, and claim purpose. It is separate from an email verification code. Requesting a claim code does not invalidate an outstanding email verification code. Requesting another claim code invalidates only the previous claim code.
- `ClaimUnverifiedAccount` accepts the email address, current claim code, and a new password that meets the signup password policy. On success, Identity consumes the code, retires the old unverified account and subject, revokes its sessions and challenges, and creates one new subject with a verified email and one new session. It returns access and refresh tokens immediately.
- The claim transition is atomic. If email verification commits first, the account is no longer eligible for claim. If claim commits first, old tokens fail the next live session check. Concurrent or repeated claims cannot create a second new account.
- A lost claim response is recovered through password login with the new password. A retry with a consumed code returns the generic claim error. The claim endpoint does not accept `Idempotency-Key`; ADR-0032 records this exception.

### Email code and verification

- Identity generates a six-digit code with a cryptographically secure random source. It binds the challenge to the account and the current email address, stores a keyed one-way verifier instead of plaintext, and sends the code only to that address.
- Each verification or claim code expires after ten minutes and can succeed once. Five wrong guesses exhaust one challenge. Identity also limits wrong guesses to ten per account per hour and 20 per account per day across challenges and service replicas.
- Requesting a new verification code invalidates the previous outstanding verification code for that address. Requesting a new claim code invalidates the previous outstanding claim code. Delivery retries must not create two concurrently valid codes for one purpose.
- A new code request must be at least 60 seconds after the previous request. Identity issues no more than five codes per account per hour across code request paths and service replicas.
- `VerifyEmail` accepts a valid code for the authenticated account only. It marks the email verified and consumes the code in one transaction. Concurrent use of the same code cannot verify two accounts or succeed twice.
- After verification commits, an Identity session check returns the current verified state for that subject. Workspace denies an unverified subject before workspace authorization and admits a verified subject only if its other access rules pass.
- An expired, guessed, or reused code never changes the verified state. `VerifyEmail` succeeds without changing state if the account is already verified. `RequestEmailVerificationCode` returns `FailedPrecondition` if the account is already verified.

### Abuse and failure handling

- Identity allows at most ten signup requests per source IP per hour. Rate limits also cover code issuance and guesses. The threat model must cover account enumeration, automated signup, email flooding, and online guessing of a six-digit code.
- Error responses and logs never contain passwords, refresh tokens, verification codes, or full verification messages. Code request and email verification responses never contain refresh tokens. Only successful signup and claim responses in this capability contain a refresh token. Security events record outcomes without storing those values.
- Identity commits the account and a durable first-email delivery request together. A delivery failure after the account commit does not change the successful signup response. Identity retries delivery, and the signed-in user can request another code after delivery recovers, subject to request limits. The threat model must cover the confidentiality and removal of queued code material.
- Mailpit captures email in learning environments. Password login, real delivery, optional MFA, independent backups, and tested restoration remain required before real users join FlowSpace.

## Commands

Run commands from the repository root. These are future implementation checks; this spec does not claim that Identity code or generated contracts already exist.

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

Use unit tests for email, password, and code validation, account state transitions, and challenge expiry. Cover ASCII email comparison, both accepted password paths, password blocklist rejection, ten-minute code expiry, and shared code limits. Cover the password boundaries: seven characters fail, eight characters need a lowercase ASCII letter and an ASCII digit, and 15 characters need no character-type mix. Use PostgreSQL integration tests for unique account creation, purpose-bound one-time code use, concurrent verification and claim, old-session revocation, session creation, and durable email delivery.

Test the public generated REST and typed RPC surfaces with API clients. Cover malformed input, unauthorized calls, duplicate signup, an ambiguous signup result, generic claim-code requests, claim codes that cannot verify email, verification codes that cannot claim an account, rate limits, code resend, stale codes, expired codes, concurrent guesses, and email-delivery failure. Prove that a newly issued token cannot use Workspace before verification, that the same live session passes the verified-email gate after verification, and that old sessions fail after a successful claim. Use Mailpit for end-to-end learning-environment evidence without real users.

## Boundaries

### Always

- Bind verification to the authenticated subject and its current email address.
- Keep unverified accounts out of Workspace even when they hold valid tokens.
- Hash passwords, protect verification codes with a keyed one-way verifier, enforce finite code lifetimes and attempt limits, and use a shared limit store when more than one replica serves requests.
- Keep secrets and full email addresses out of application logs and traces.
- Measure the approved Argon2id configuration on the deployment host before real users join.

### Ask first

- Obtain approval before changing the token issuance outcome or adding an email-change flow.

### Never

- Do not treat possession of an access token as proof of email ownership.
- Do not accept a client-supplied subject or email as authority for verification.
- Do not use an email address as the stable subject or link accounts by matching email addresses.

## Success criteria

Each row describes an observable result required before implementation can claim completion.

| Given | Then |
| --- | --- |
| A new user submits a valid email address and password. | Identity creates one unverified account and one session, requests an email code, and returns a stable subject plus access and refresh tokens. |
| A user tries to sign up with an email address that already belongs to an account. | Identity returns `AlreadyExists` (HTTP 409) without tokens. |
| A user loses the signup response but knows the password. | A retry returns `AlreadyExists` if signup committed. Once login exists, the user logs in and obtains new tokens without changing the subject. |
| An email owner does not know the password of an unverified signup. | The owner requests a purpose-bound claim code, supplies it with a new password, and receives a verified new subject and session. The old subject and sessions stop working. |
| A client requests a claim code for a missing or already verified account. | Identity returns the same accepted response as for an eligible account and sends no code. |
| The new user calls Workspace with the access token before email verification. | Workspace rejects the request because Identity reports that the email is unverified. |
| The authenticated user submits the valid current code. | Identity consumes the code once and reports that the email is verified. A later session check reports the new state without replacing the tokens. |
| An unverified user submits a malformed, wrong, expired, replaced, or consumed code. | Identity does not verify the email and returns the contract's safe error. |
| The user requests another code. | Only the newest challenge remains valid, subject to the approved request limits. |
| Two requests submit one valid code at the same time. | At most one request consumes the code; the account ends in one verified state. |
| A client submits a wrong, expired, used, or ineligible claim code. | Identity returns one generic `InvalidArgument` error without changing an account. |
| Email verification races with an account claim. | One transition wins. Verification prevents claim; claim revokes the old subject and its sessions. |
| Email delivery fails after account creation. | Signup still returns tokens, the account remains unverified, and durable delivery retries continue. |
| A request exceeds a signup or code limit. | Identity rejects the request without issuing another code or verifying the account. |
| The capability is submitted for implementation review. | Contract, integration, abuse, and workspace-gate tests pass under repository quality checks. |

## Open questions and approval

The owner approved this spec, including the claim API, ADR-0032, and the first-release Argon2id parameters. No approval question remains for planning. Before real users join, benchmark those parameters on the deployment host under expected login load and record the results. If one hash takes at least one second or the workload exhausts CPU or memory, do not admit real users until the configuration is reviewed and the benchmark passes. Approval permits planning; it does not mean that the capability is implemented or ready for real users.
