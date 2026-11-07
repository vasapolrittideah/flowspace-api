# ADR-0053: Browser provider flows redirect the handoff code to the frontend

Date: 2026-11-07

Status: Accepted; scoped exceptions to [ADR-0034](0034-provider-login-uses-a-one-time-handoff.md) and [ADR-0045](0045-provider-re-authentication-reuses-the-one-time-handoff.md) and [ADR-0046](0046-provider-link-reuses-the-one-time-handoff.md)

## Context

The web frontend needs Google and GitHub login in the browser. The user must not copy a code between 2 pages.

[ADR-0034](0034-provider-login-uses-a-one-time-handoff.md) shows a one-time handoff code on an Identity callback page, and the API client submits it with a secret attempt token. [ADR-0045](0045-provider-re-authentication-reuses-the-one-time-handoff.md) and [ADR-0046](0046-provider-link-reuses-the-one-time-handoff.md) reuse the same handoff for provider re-authentication and provider link. The provider sign-in replaces the frontend page, so the attempt token in browser memory is lost before the user comes back.

ADR-0034 leaves the token handling of a later web client to its own record. [ADR-0052](0052-browser-sessions-keep-the-refresh-token-in-an-httponly-cookie.md) gives browser clients an HttpOnly refresh-token cookie and an exact list of allowed frontend origins.

## Decision

For browser clients, Identity keeps the attempt token in an HttpOnly cookie and redirects the provider callback to the frontend with the handoff code. API clients keep the callback page of ADR-0034.

Each RPC that starts a provider attempt accepts an optional return URL. When the request has a return URL, Identity compares it byte for byte, without URL normalization, with a list of frontend callback URLs, which the deployment configures for each environment. A configured URL has no fragment. Identity rejects a URL that is not on the list with `InvalidArgument`. Identity stores the return URL with the attempt. It sets the attempt token in a cookie and leaves the `attempt_token` field of the response empty.

Each purpose has its own attempt cookie: provider login, provider re-authentication, and provider link. Each attempt cookie has the same attributes as the refresh-token cookie of ADR-0052, including the `__Host-` prefix, but it has its own name. Identity reads each attempt cookie only in the route that completes an attempt of that purpose. The cookie ends when the attempt expires.

After a successful callback, Identity answers with HTTP 303 to the stored return URL. The response has the `Cache-Control: no-store` and `Referrer-Policy: no-referrer` headers, as the callback page has. The handoff code is in the URL fragment, such as `#handoff_code=<code>`. After a failed callback, the fragment holds only a safe error code. If the callback cannot find its attempt, such as for an unknown or expired `state`, Identity has no return URL and shows the callback page of ADR-0034 with a safe error and no handoff code. The browser sends no fragment to a server or in a `Referer` header. The frontend reads the fragment and removes it from the address bar before it loads a third-party script or follows a redirect.

When a request that completes a provider attempt has an empty `attempt_token` field, Identity reads the attempt token from the cookie. A completion that reads the attempt token from the cookie always uses the cookie delivery of ADR-0052 for each refresh token that it issues. Identity sends an expired attempt cookie after a successful completion and after a failure that ends the attempt. This has no effect on an API client. The `Origin` and content type checks of ADR-0052 apply to a start request that has a return URL and to a completion request that Identity reads from the attempt cookie.

This is a scoped exception to ADR-0034, ADR-0045, and ADR-0046 for attempts that have a return URL. The callback adapter redirects instead of showing the handoff page. The 2 proofs, the expiry, and the single use of the handoff code stay the same.

## Alternatives Considered

### A popup window with `postMessage`

- Pros: the main page keeps the attempt token in memory, and Identity needs no attempt cookie.
- Cons: browsers block popups that do not come from a direct click, and popups work poorly on mobile browsers.
- Rejected: a full-page redirect works in each browser, and the cookie keeps the attempt token out of scripts.

### The attempt token in browser session storage

- Pros: Identity needs no attempt cookie, and the token survives the redirect.
- Cons: an injected script can read the attempt token and finish the attempt with a stolen handoff code.
- Rejected: an HttpOnly cookie gives the same redirect behavior without a token that scripts can read.

### The callback creates the session

- Pros: the frontend makes no completion call, because the callback sets the refresh-token cookie and redirects without a code.
- Cons: the `SameSite=Strict` attempt cookie does not reach the cross-site callback request, and the callback issues sessions outside the Protobuf RPCs of ADR-0005.
- Rejected: a completion RPC from the frontend keeps session issuance in the contract and receives the attempt cookie as a same-site request.

### The handoff code in the query string

- Pros: the frontend server can read the code during the first request.
- Cons: the code goes into server logs, hosting logs, and `Referer` headers.
- Rejected: the fragment stays in the browser, and the frontend reads it in client code.

## Consequences

- A browser user signs in, re-authenticates, or links a provider with Google or GitHub without copying a code.
- A handoff code works only in the browser that started its attempt, because that browser holds the attempt cookie. An attacker cannot sign a victim in to the attacker's account with a crafted redirect link. Another host of the registrable domain cannot plant an attempt cookie, because of the `__Host-` prefix, but a script on the Identity host can.
- Each provider start RPC gets a return URL field, and the `attempt_token` field of each completion request stops being required.
- A second attempt of the same purpose in the same browser, such as in another tab, replaces the attempt cookie of the first attempt, and the first attempt can no longer complete.
- For browser clients, this record replaces the rules of the [Identity provider login](../specs/identity-provider-login.md) specification that keep the attempt token in the API client, forbid a redirect target from the client, forbid a handoff code in a redirect URL, show the code only on the callback page, and require separate browser and API-client proof. The [Identity re-authentication](../specs/identity-re-authentication.md) and [Identity provider link](../specs/identity-provider-link.md) specifications reuse that callback page. These specifications must change before implementation.
- A frontend deployment on another site, such as a Vercel preview domain, gets no attempt cookie and cannot use provider flows.
- A wrong list of frontend callback URLs sends the handoff code to the wrong page or blocks provider flows. The deployment configuration of each environment must name exact URLs.
- Open work: the Identity threat model review of the return URL list and the fragment handoff, the return URL field name, the attempt cookie names, the safe error codes, and the frontend callback URLs for each environment. The provider login, re-authentication, and link specifications decide them.

## Sources

- [RFC 9700: Best Current Practice for OAuth 2.0 Security](https://www.rfc-editor.org/rfc/rfc9700)
