# ADR-0045: Provider re-authentication reuses the one-time handoff

Date: 2026-10-16

Status: Accepted; scoped exceptions to [ADR-0005](0005-one-protobuf-contract-generates-rest.md) and [ADR-0011](0011-idempotency-keys-protect-non-idempotent-creates.md)

## Context

Identity will ask a signed-in user to prove their identity again before a sensitive action, such as unlinking a provider or deleting the account. A provider-only account has no password, so it needs Google or GitHub to give this proof. Like provider login, the proof arrives at a browser callback, while the API client makes the sensitive request.

[ADR-0034](0034-provider-login-uses-a-one-time-handoff.md) gives provider login a browser callback, a one-time handoff code, and a secret attempt token. Its exceptions to [ADR-0005](0005-one-protobuf-contract-generates-rest.md) and [ADR-0011](0011-idempotency-keys-protect-non-idempotent-creates.md) apply only to `StartProviderLogin` and `CreateProviderSession`. No record states how a signed-in session receives provider proof.

## Decision

Provider re-authentication uses the callback and the handoff of ADR-0034 for a session that is already signed in. `StartProviderReauthentication` and `CompleteProviderReauthentication` are Protobuf RPCs with generated REST/JSON routes, and both need the bearer access token of the session.

`StartProviderReauthentication` creates an attempt that is bound to the subject and the session of the access token. It returns a provider authorization URL and a secret attempt token. The provider returns to the same callback route as provider login, and the attempt decides the purpose of the callback. The callback page shows a one-time handoff code and no Flowspace token. The callback adapter is a scoped exception to ADR-0005, as in ADR-0034.

`CompleteProviderReauthentication` takes the attempt token and the handoff code from the same session. It succeeds only when the provider subject matches a provider link of that subject. It issues no token, and it changes no account, link, or session other than the re-authentication time of the current session.

`StartProviderReauthentication` rejects `Idempotency-Key`, as `StartProviderLogin` does. This is a scoped exception to ADR-0011. If the start response is lost, the client starts a new attempt and the old one expires.

## Alternatives Considered

### A separate callback route for re-authentication

- Pros: each callback route has 1 purpose, so the adapter does not look up the purpose of the attempt.
- Cons: Google and GitHub need a second registered redirect URI in each environment.
- Rejected: 1 registered callback route with a purpose on the attempt keeps the provider setup the same as provider login.

### Provider login creates a new session as the proof

- Pros: re-authentication needs no new RPCs, because the client runs provider login again.
- Cons: each re-authentication creates a new session and token pair, and the old session stays active without the proof.
- Rejected: the proof belongs to the session that makes the sensitive request, and a new session does not prove that this session is still in the hands of its owner.

### Password or email code only

- Pros: Identity needs no new provider flow, and the client needs no browser step.
- Cons: a provider-only account with no verified email cannot re-authenticate, so it cannot do any sensitive action.
- Rejected: every account type needs a re-authentication method, and provider proof is the one method that each provider-only account has.

## Consequences

- API-client users copy a handoff code from the callback page again, as in provider login.
- The provider attempt store holds a purpose, a subject, and a session ID in addition to the values of ADR-0034. A provider result for one purpose cannot complete the other purpose.
- A lost `CompleteProviderReauthentication` response leaves the session re-authenticated. The client can continue with the sensitive action or start a new attempt.
- The Identity re-authentication specification defines the time window, the limits, the other methods, and the tests for this flow.
