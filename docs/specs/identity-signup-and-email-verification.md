# Spec: Identity signup and email verification

Module id: `identity-signup-and-email-verification`

Status: Draft.

## Objective

Allow a new FlowSpace user to create an account with an email address and password, receive an email code, and prove control of that address. The first clients are API clients using disposable data. A successful signup returns an access token and a refresh token immediately, but the unverified account cannot use Workspace.

This spec defines the account creation and email verification flow. It does not report implementation progress or readiness to serve real users.

## Scope and decision sources

The scope covers signup, the first verification email, requesting a new code, consuming a code, and the verified-email state that protected services receive from Identity. Account creation and verification form one capability because signup creates the unverified state and its first verification challenge.

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

The contract uses canonical gRPC errors and the default HTTP mapping. Invalid request shape, malformed email, malformed code, and a password outside the approved policy use `InvalidArgument` (HTTP 400) with safe field details. Missing or invalid authentication uses `Unauthenticated` (HTTP 401). Incorrect, expired, or consumed codes share one safe error category and message, to be fixed before approval. Rate limits use `ResourceExhausted` (HTTP 429). Internal and email-delivery failures do not expose account data or infrastructure details.

The duplicate-email response and the exact delivery-failure response are open questions. They must be fixed before this contract is approved. A failed or ambiguous signup response does not authorize clients to create another account blindly.

## Required behavior

### Account creation

- Identity assigns a stable subject that does not depend on the email address. It stores the account and an unverified email state in its own PostgreSQL database.
- Identity enforces uniqueness using one documented email comparison rule. The exact normalization rule remains open; implementations must not invent provider-specific rules such as removing dots from an address.
- Identity validates the email and password at the public boundary. It stores a salted, adaptive password hash and never stores or returns the password.
- Identity accepts a password with at least 15 characters without requiring any character type. It also accepts a password with 8–14 characters if it contains at least one lowercase ASCII letter (`a`–`z`) and one ASCII digit (`0`–`9`). It accepts Unicode and counts Unicode code points when measuring length. It rejects passwords shorter than eight characters. These two length paths follow [GitHub's policy](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-strong-password). They depart from [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html#passwordver): MFA is optional for the shorter path, and that path requires a character-type mix.
- Successful signup creates one device session and returns its access and refresh tokens even though the email is not verified.
- A signup retry after a lost response must not create a second account or session by accident. The retry contract remains open because the first response contains a refresh token that Identity stores only as a hash.

### Email code and verification

- Identity generates a six-digit code with a cryptographically secure random source. It binds the challenge to the account and the current email address, stores a keyed one-way verifier instead of plaintext, and sends the code only to that address.
- A code expires, has a bounded number of guesses, and can succeed once. Identity applies limits to code requests and guesses across service replicas. Exact values remain open.
- Requesting a new code invalidates the previous outstanding code for that address. Delivery retries must not create two concurrently valid codes.
- Identity accepts a valid code for the authenticated account only. It marks the email verified and consumes the code in one transaction. Concurrent use of the same code cannot verify two accounts or succeed twice.
- After verification commits, an Identity session check returns the current verified state for that subject. Workspace denies an unverified subject before workspace authorization and admits a verified subject only if its other access rules pass.
- An expired, guessed, or reused code never changes the verified state. An already verified account does not create a new verification challenge.

### Abuse and failure handling

- Rate limits cover public signup, code issuance, and code guesses. The threat model must cover account enumeration, automated signup, email flooding, and online guessing of a six-digit code.
- Error responses and logs never contain passwords, refresh tokens, verification codes, or full verification messages. The successful signup response is the only response in this capability that contains a refresh token. Security events record outcomes without storing those values.
- A failed email delivery leaves the account unverified. The user must have a defined path to request another code after delivery recovers. The response and retry behavior remain open.
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

Use unit tests for email, password, and code validation, account state transitions, and challenge expiry. Cover both accepted password paths and their boundaries: seven characters fail, eight characters need a lowercase ASCII letter and an ASCII digit, and 15 characters need no character-type mix. Use PostgreSQL integration tests for unique account creation, one-time code use, concurrent verification, session creation, and durable retry behavior after the retry contract is settled.

Test the public generated REST and typed RPC surfaces with API clients. Cover malformed input, unauthorized calls, duplicate signup, rate limits, code resend, stale codes, expired codes, concurrent guesses, and email-delivery failure. Prove that a newly issued token cannot use Workspace before verification and that the same live session passes the verified-email gate after verification. Use Mailpit for end-to-end learning-environment evidence without real users.

## Boundaries

### Always

- Bind verification to the authenticated subject and its current email address.
- Keep unverified accounts out of Workspace even when they hold valid tokens.
- Hash passwords, protect verification codes with a keyed one-way verifier, enforce finite code lifetimes and attempt limits, and use a shared limit store when more than one replica serves requests.
- Keep secrets and full email addresses out of application logs and traces.

### Ask first

- Approve the email comparison rule, password policy, signup retry behavior, duplicate-email response, delivery-failure response, and numeric abuse limits before this spec becomes Approved.
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
| The new user calls Workspace with the access token before email verification. | Workspace rejects the request because Identity reports that the email is unverified. |
| The authenticated user submits the valid current code. | Identity consumes the code once and reports that the email is verified. A later session check reports the new state without replacing the tokens. |
| The user submits a malformed, wrong, expired, replaced, or consumed code. | Identity does not verify the email and returns the contract's safe error. |
| The user requests another code. | Only the newest challenge remains valid, subject to the approved request limits. |
| Two requests submit one valid code at the same time. | At most one request consumes the code; the account ends in one verified state. |
| Email delivery fails after account creation. | The account remains unverified and the approved retry path can deliver a new code after recovery. |
| A request exceeds a signup or code limit. | Identity rejects the request without issuing another code or verifying the account. |
| The capability is submitted for implementation review. | Contract, integration, abuse, and workspace-gate tests pass under repository quality checks. |

## Open questions and approval

- What email syntax and comparison rule does FlowSpace accept, including Unicode addresses and case handling?
- Which adaptive hash parameters must pass before real-user use?
- How long does a verification code live, how many guesses are allowed, and what request limits and resend interval apply?
- How do signup retries recover a lost response without storing or exposing a reusable plaintext refresh token? What does duplicate signup return?
- What does signup return when email delivery fails after account creation, and how does the client recover?
- Which canonical error category covers a wrong, expired, or used code, and what does an already verified account return?

The owner must approve these answers and the final API contract before this Draft becomes Approved. Approval permits planning; it does not mean that the capability is implemented or ready for real users.
