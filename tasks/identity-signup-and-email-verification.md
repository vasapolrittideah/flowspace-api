# Implementation plan: Identity signup and email verification

Module id: `identity-signup-and-email-verification`

Status: Approved.

## Overview

Build account signup, email verification, and recovery of an unverified address through an account claim. The [approved specification](../docs/specs/identity-signup-and-email-verification.md) defines this flow. Signup returns one session and token pair after a database commit. The account, first challenge, and email outbox record commit together. An Identity worker sends the email after an outbox relay publishes a delivery request through Redpanda. Broker or Mailpit failure delays delivery without changing a committed signup result.

The first clients use disposable data. Password login, refresh, logout, provider login, password reset, and real email delivery belong to other capabilities. Tasks are tracked in the [flowspace-api GitHub Project](https://github.com/users/vasapolrittideah/projects/4).

## Architecture decisions

- The `flowspace.identity.v1` Protobuf contract owns the five public RPCs and generated REST routes. Generated files contain no business rules.
- Identity owns account, session, challenge, limit, and outbox data in its PostgreSQL instance. A database uniqueness constraint enforces the approved email comparison rule under concurrent signup and claim requests.
- The account subject is stable and separate from the email address. Only a validated access token identifies the account for resend and verification.
- Password rules, Argon2id parameters, code limits, and session lifetimes come from the approved Identity specifications. Task 1 records the still open signing, key, source-address, blocklist, and queued-code protection choices before code depends on them.
- The signup transaction stores the unverified account, one session, one purpose-bound challenge, and one outbox record. It stores only a keyed verifier for code checks. Encrypted delivery material stays in Identity storage until delivery or expiry; the Redpanda event carries opaque identifiers, not an email address or code.
- The relay republishes an outbox record with the same event ID until Redpanda confirms it. The Identity email worker checks that the challenge is still current before sending to Mailpit, records its outcome, then commits the broker offset. A crash after Mailpit accepts a message can resend the same code; the system does not promise exactly one email.
- Resend and claim-code requests use the same durable delivery path. Replacing a challenge invalidates only the previous code for that purpose, and a stale delivery request cannot send an old code.
- Shared PostgreSQL limit state enforces the approved per-source and per-account limits across Identity replicas. The request handler uses only a trusted proxy source address.
- Workspace keeps its own membership rules. Its admission path asks Identity for the live session and verified-email state before it checks membership. This work must coordinate with the separate password-session capability.

## Dependency graph

```mermaid
flowchart TD
    Decisions[Approve open security choices] --> Contract[Identity API contract]
    Decisions --> Rules[Email, password, and code rules]
    Decisions --> Storage[Identity schema and typed SQL]
    Rules --> Signup[Atomic signup and first outbox record]
    Storage --> Limits[Shared request and code limits]
    Storage --> Tokens[Session and token issuance]
    Contract --> Signup
    Limits --> Signup
    Tokens --> Signup
    Signup --> Relay[Outbox relay to Redpanda]
    Relay --> Mail[Identity email worker to Mailpit]
    Mail --> Resend[Request another verification code]
    Resend --> Verify[Verify email]
    Mail --> ClaimCode[Request an account claim code]
    ClaimCode --> Claim[Claim an unverified account]
    Verify --> Runtime[Identity API and workers]
    Claim --> Runtime
    Runtime --> Local[Local cluster resources]
    Verify --> Gate[Workspace verified-email gate]
    Claim --> Gate
    Local --> Evidence[Final public and failure evidence]
    Gate --> Evidence
```

## Task list

### Phase 1: Decisions and foundations

- Task 1: [#100 Approve the remaining signup security choices](https://github.com/vasapolrittideah/flowspace-api/issues/100)
- Task 2: [#101 Define the public Identity contract](https://github.com/vasapolrittideah/flowspace-api/issues/101)
- Task 3: [#102 Implement email, password, and code rules](https://github.com/vasapolrittideah/flowspace-api/issues/102)
- Task 4: [#103 Add Identity schema and typed queries](https://github.com/vasapolrittideah/flowspace-api/issues/103)

### Checkpoint: Foundations

- [x] A human approves the security choices and the outbox treatment of code material.
- [x] Buf accepts the public contract and generated output, and sqlc generates from the Identity schema.
- [x] Domain tests cover the approved email, password, and code boundaries without infrastructure.

### Phase 2: Durable signup and asynchronous email

- Task 5: [#104 Enforce shared signup and code limits](https://github.com/vasapolrittideah/flowspace-api/issues/104)
- Task 6: [#105 Issue the first device session and tokens](https://github.com/vasapolrittideah/flowspace-api/issues/105)
- Task 7: [#106 Commit signup and its first delivery request](https://github.com/vasapolrittideah/flowspace-api/issues/106)
- Task 8: [#107 Publish Identity outbox requests to Redpanda](https://github.com/vasapolrittideah/flowspace-api/issues/107)
- Task 9: [#108 Deliver current codes through Mailpit](https://github.com/vasapolrittideah/flowspace-api/issues/108)

### Checkpoint: Signup and delivery

- [x] A valid signup returns one unverified subject and one token pair after its database transaction commits.
- [x] Broker and Mailpit outages leave a retryable delivery request and do not turn a committed signup into a failed response.
- [x] Broker records, logs, and traces contain no email code, password, token, or full email address.

### Phase 3: Verification and account claim

- Task 10: [#109 Request another verification code](https://github.com/vasapolrittideah/flowspace-api/issues/109)
- Task 11: [#110 Verify the authenticated email address](https://github.com/vasapolrittideah/flowspace-api/issues/110)
- Task 12: [#111 Request an unverified-account claim code](https://github.com/vasapolrittideah/flowspace-api/issues/111)
- Task 13: [#112 Claim an unverified account](https://github.com/vasapolrittideah/flowspace-api/issues/112)

### Checkpoint: Account state

- [x] One current code succeeds once, with the approved expiry and guess limits, even under concurrent requests.
- [x] Claim and verification races produce one valid account state; a successful claim retires the old subject and sessions.
- [x] Missing and ineligible claim-code requests return the same accepted response without an email.

### Phase 4: Runtime and completion evidence

- Task 14: [#113 Start the Identity API and workers](https://github.com/vasapolrittideah/flowspace-api/issues/113)
- Task 15: [#114 Deploy Identity in the local cluster](https://github.com/vasapolrittideah/flowspace-api/issues/114)
- Task 16: [#115 Run Redpanda and Mailpit in the local cluster](https://github.com/vasapolrittideah/flowspace-api/issues/115)
- Task 17: [#116 Deny Workspace access until email verification](https://github.com/vasapolrittideah/flowspace-api/issues/116)
- Task 18: [#117 Prove the complete public and failure paths](https://github.com/vasapolrittideah/flowspace-api/issues/117)

### Checkpoint: Complete

- [ ] Public REST and typed RPC tests cover every success criterion and applicable threat ID in the specification.
- [ ] Generated files match source contracts and queries. Required repository checks pass without weaker settings.
- [ ] The review diff contains no unrelated changes or secrets, and the module is ready for maintainer review.

## Risks and controls

| Risk | Impact | Control |
| --- | --- | --- |
| Redpanda or Mailpit is unavailable after signup commits. | The first email arrives late. | Keep the delivery request in the same transaction as signup, retry publication and delivery, and prove recovery in integration tests. |
| A broker replay or worker crash sends a message twice. | The user can receive duplicate mail. | Keep one valid challenge per purpose, use a stable event ID, and send the same current code on a delivery retry. Do not claim exactly one email. |
| A replaced or expired challenge still has a queued event. | The user can receive a stale code. | The worker checks current purpose, challenge ID, and expiry before sending; it discards stale work and removes delivery material. |
| Stored delivery material or broker records disclose a code. | An attacker can verify or claim an account. | Approve key storage, encryption, retention, and cleanup first. Put only opaque identifiers on Redpanda and test telemetry for secrets. |
| Verification races with claim or concurrent code use. | Two subjects or state transitions can become valid. | Serialize the affected account in PostgreSQL and test competing transactions. |
| The password-session capability changes shared session code. | Signup and Workspace admission can drift. | Reuse its approved session contract and coordinate one implementation of token issuance and live session checks. |
