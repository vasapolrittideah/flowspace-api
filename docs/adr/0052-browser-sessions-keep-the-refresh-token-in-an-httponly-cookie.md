# ADR-0052: Browser sessions keep the refresh token in an HttpOnly cookie

Date: 2026-11-07

Status: Accepted; scoped exception to [ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md)

## Context

The web frontend calls the public Go APIs directly from the browser with a bearer access token. It has no backend-for-frontend server that can hold a session for it. After a page reload, the browser must get a new access token without a new login. The browser must also be able to call the APIs from a frontend origin that differs from the API origin.

Identity returns the access token and the refresh token only in the JSON body of each token-issuance response, and it sets no cookies. A refresh token that only lives in browser memory is lost on each reload. A refresh token in browser storage that scripts can read lets an injected script take a reusable credential. The public APIs send no Cross-Origin Resource Sharing (CORS) headers, so the browser blocks each cross-origin call.

[ADR-0031](0031-flowspace-owns-authentication-and-revocable-sessions.md) and [ADR-0034](0034-provider-login-uses-a-one-time-handoff.md) leave browser token storage to the web release. The [Open proposals](../architecture.md#open-proposals) section of the architecture leaves the public hostnames open. [ADR-0026](0026-an-outbound-tunnel-exposes-selected-routes.md) puts staging behind an edge access policy.

## Decision

For browser clients, Identity sends the refresh token in an HttpOnly cookie instead of the JSON body, and the access token stays in the JSON body and in browser memory. API clients keep the current JSON-only behavior.

Each RPC that issues a refresh token accepts a request field that selects cookie delivery. The default stays JSON delivery. When the client selects cookie delivery, Identity leaves the `refresh_token` field of the response empty and sets the cookie. The `refresh_token_expires_at` field still shows the expiry.

The cookie has these attributes:

- The name has the `__Host-` prefix, so the browser rejects a cookie with the same name from another host of the registrable domain.
- The cookie has the `Secure`, `HttpOnly`, and `SameSite=Strict` attributes.
- The cookie has no `Domain` attribute, so the browser sends it only to the host that set it.
- The `Path` attribute is `/`, as the `__Host-` prefix requires.
- The `Max-Age` attribute ends the cookie at the expiry of the refresh token.

In staging and production, Identity has its own hostname, such as `auth.<domain>`, so the browser sends the cookie to no other service. In a browser, the cookie also reaches each Identity route that the frontend calls with credentials, but Identity reads it only in `RefreshSession`. This is a scoped exception to the rule of ADR-0031 that clients send refresh tokens only to the refresh endpoint.

When a `RefreshSession` request has an empty `refresh_token` field, Identity reads the refresh token from the cookie. A refresh that reads the refresh token from the cookie always uses cookie delivery, whatever the request field says, and leaves the `refresh_token` field of the response empty. A successful refresh rotates the token and sets a new cookie. Logout of the current session, logout of all sessions, and a refresh that fails with `Unauthenticated` always send an expired cookie, which clears the cookie in a browser and has no effect on an API client. The rotation and reuse rules of ADR-0031 apply to the cookie in the same way as to a JSON refresh token.

The `Origin` and content type checks apply to a request that selects cookie delivery and to a `RefreshSession` request that Identity reads from the cookie. Identity rejects such a request with `PermissionDenied` when its `Origin` header is not exactly equal to an allowed frontend origin. A missing `Origin` header or the `null` value fails the check. The request must also have the `application/json` content type. That content type makes the browser send a CORS preflight request first, so a form on another site cannot send the request.

In staging and production, the frontend and the public API must use hostnames under the same registrable domain, such as `app.<domain>` for the frontend and `auth.<domain>` for Identity. The browser then treats the cookie as a first-party cookie. Locally, `localhost` on different ports is the same site. Local development supports only browsers that treat `http://localhost` as a secure origin for `Secure` cookies, such as Chrome and Firefox.

Each public Go service answers CORS requests only for an exact list of frontend origins, which the deployment configures for each environment. The list has no wildcards. The services allow the `GET`, `POST`, and `PATCH` methods and the `Authorization`, `Content-Type`, `Idempotency-Key`, and `X-Request-ID` request headers, and they expose the `X-Request-ID` response header. Each response whose `Access-Control-Allow-Origin` header depends on the request `Origin` has the `Vary: Origin` header. Only Identity allows credentials, because only Identity reads or sets cookies.

## Alternatives Considered

### A refresh token in browser memory only

- Pros: Identity needs no change, and no script can read a stored credential after a reload.
- Cons: each reload or new tab ends the browser session, so the user signs in again.
- Rejected: an HttpOnly cookie keeps the session across reloads and still hides the refresh token from scripts.

### A `__Secure-` cookie with a narrow path

- Pros: the cookie goes only to the refresh route, and Identity can share 1 hostname with the other services.
- Cons: another host of the registrable domain can set a cookie with the same name and sign the victim in to the attacker's session.
- Rejected: the `__Host-` prefix stops that cookie from other hosts, and a separate Identity hostname keeps the cookie away from other services.

### A refresh token in browser storage

- Pros: Identity needs no change, and the session survives a reload.
- Cons: an injected script can read a refresh token that stays valid for up to 30 days.
- Rejected: an HttpOnly cookie gives the same reload behavior without a credential that scripts can read.

### A Next.js backend-for-frontend

- Pros: the browser uses 1 origin, and the frontend server keeps the tokens.
- Cons: the frontend needs a second session life cycle and a server that forwards each API call.
- Rejected: the frontend calls the Go APIs directly, and a cookie from Identity needs no extra server.

### A cross-site cookie with `SameSite=None`

- Pros: the frontend and the API can use unrelated domains, such as a Vercel domain.
- Cons: browsers block third-party cookies more and more, and the cookie needs stronger cross-site request forgery protection.
- Rejected: a shared registrable domain makes the cookie first-party and lets it use `SameSite=Strict`.

## Consequences

- A reload restores the browser session with 1 `RefreshSession` call, and no script can read the refresh token.
- A cross-site scripting flaw can still use the in-memory access token and call `RefreshSession` while the page runs. The flaw cannot take the refresh token away from the browser.
- Each token-issuance RPC and `RefreshSession` get a new request field, and the `refresh_token` field of `RefreshSessionRequest` stops being required. The public REST/JSON routes stay the same.
- The public API proxy must forward the `Cookie` request header to Identity and turn Identity response metadata into a `Set-Cookie` header. The edge, the tunnel, and the proxy must keep the `Cookie` and `Set-Cookie` headers out of logs, spans, and errors.
- The browser sends the cookie with each Identity request that includes credentials. The frontend includes credentials only in the requests that read or set the cookie.
- Locally, cookies ignore the port, so the browser sends the refresh-token and attempt cookies to the frontend development server and to the other services on `localhost`. Local data is disposable, as [ADR-0022](0022-single-host-storage-holds-disposable-data.md) states, and local logs must still omit the `Cookie` header.
- The frontend keeps a separate base URL for Identity, because Identity does not share the public origin of the other services.
- The edge and the tunnel must expose the Identity hostname, and the Google and GitHub redirect URIs of each environment use that hostname. The Identity hostname serves only Identity routes and the callback page, with no user content and no third-party scripts.
- A frontend deployment on another site, such as a Vercel preview domain, gets no cookie. That deployment can still sign in, but a reload needs a new login.
- The staging edge access policy must admit CORS preflight requests and credentialed requests from the staging frontend.
- Tabs of 1 browser share 1 refresh-token cookie. If 2 tabs refresh at the same time, Identity sees the second refresh as reuse and revokes the session, as ADR-0031 states. The frontend must allow only 1 refresh at a time across tabs, for example with the Web Locks API.
- The web frontend must replace its record that keeps all tokens in memory, because the refresh token moves to a cookie.
- For browser clients, this record replaces the rule of the [Identity password login and sessions](../specs/identity-password-login-and-sessions.md) specification that each token-issuance response contains `refresh_token`. That specification must change before implementation.
- Open work: the Identity threat model review of cookies and cross-site request forgery, the exact hostnames, the cookie name, the allowed origins for each environment, and the remembered-device token of [ADR-0050](0050-remembered-devices-skip-the-mfa-login-step-for-30-days.md) in the browser. A specification or a later record decides them.

## Sources

- [RFC 6265bis: Cookies](https://datatracker.ietf.org/doc/draft-ietf-httpbis-rfc6265bis/)
- [Fetch Standard: CORS protocol](https://fetch.spec.whatwg.org/#http-cors-protocol)
