# ADR-0046: Provider link reuses the one-time handoff

Date: 2026-10-17

Status: Accepted; scoped exceptions to [ADR-0005](0005-one-protobuf-contract-generates-rest.md) and [ADR-0011](0011-idempotency-keys-protect-non-idempotent-creates.md)

## Context

A signed-in user will link a Google or GitHub account to their Flowspace account, so that the provider account signs in to it. [ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) states that a signed-in user must link a provider explicitly and that matching email addresses never link accounts. Like provider login, the proof of the provider account arrives at a browser callback, while the API client makes the request.

[ADR-0034](0034-provider-login-uses-a-one-time-handoff.md) gives provider login a browser callback, a one-time handoff code, and a secret attempt token. [ADR-0045](0045-provider-re-authentication-reuses-the-one-time-handoff.md) applies the same handoff to provider re-authentication, and that flow changes no link. The exceptions of both records to [ADR-0005](0005-one-protobuf-contract-generates-rest.md) and [ADR-0011](0011-idempotency-keys-protect-non-idempotent-creates.md) apply only to their own RPCs. No record states how a signed-in session adds a provider link.

## Decision

Provider link uses the callback and the handoff of ADR-0034 for a session that is already signed in. `StartProviderLink` and `CompleteProviderLink` are Protobuf RPCs with generated REST/JSON routes, and both need the bearer access token of the session.

`StartProviderLink` creates an attempt with the link purpose that is bound to the subject and the session of the access token. It returns a provider authorization URL and a secret attempt token. The provider returns to the same callback route as provider login and re-authentication, and the attempt decides the purpose of the callback. The callback page shows a one-time handoff code and no Flowspace token. The callback adapter is a scoped exception to ADR-0005, as in ADR-0034.

`CompleteProviderLink` takes the attempt token and the handoff code from the same session. It creates a link from the provider name and the provider subject that the callback proved. It issues no token, it creates and revokes no session, and it does not change the email address or the verified-email state of the account. The proof of the new provider account is not a re-authentication of the session.

`StartProviderLink` rejects `Idempotency-Key`, as `StartProviderLogin` does. This is a scoped exception to ADR-0011. If the start response is lost, the client starts a new attempt and the old one expires.

## Alternatives Considered

### A separate callback route for provider link

- Pros: each callback route has 1 purpose, so the adapter does not look up the purpose of the attempt.
- Cons: Google and GitHub need another registered redirect URI in each environment.
- Rejected: 1 registered callback route with a purpose on the attempt keeps the provider setup the same as provider login and re-authentication.

### A link during provider login

- Pros: the client runs provider login, and Identity links the provider to the account that owns the provider email.
- Cons: a person who controls a provider account with a matching email gains the Flowspace account.
- Rejected: ADR-0031 forbids a link from matching email addresses, and a link from the signed-in session needs no email match.

### A link from 2 sessions

- Pros: the client signs in with the provider in a second session, and Identity merges that session into the first account.
- Cons: provider login with a new provider identity creates a new account, so Identity must merge or delete accounts.
- Rejected: the attempt of the signed-in session adds the link to its own account, and no second account exists.

## Consequences

- API-client users copy a handoff code from the callback page again, as in provider login and re-authentication.
- The provider attempt store has 3 purposes: login, re-authentication, and link. A provider result for 1 purpose cannot complete an attempt of another purpose.
- A lost `CompleteProviderLink` response leaves the link in place. The client can sign in with the provider, or start a new attempt.
- The Identity provider link specification defines the re-authentication rule, the link conflicts, the limits, and the tests for this flow.
