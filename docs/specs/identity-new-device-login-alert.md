# Spec: Identity new device login alert

Module ID: `identity-new-device-login-alert`

Status: Approved

## Objective

Tell the owner of an account by email when someone signs in to the account from a device or a network that the account has not used recently. The owner then notices a stolen password or provider account early and can end the session or change the password. The first users are API clients, and all data is disposable under [ADR-0022](../adr/0022-single-host-storage-holds-disposable-data.md).

To know which devices are new, Identity keeps a short record of the known devices of each account. A known device holds no full IP address and no user agent.

## Scope and ADRs

The scope covers the known devices of each account, the decision whether a login comes from a new device, and the notice for a new device.

This capability excludes these items:

- Notices for failed logins, provider links, or session revocations.
- A change to the notices of other specifications, such as the password-change, email-change, or account deletion notices.
- A setting that turns the notice off.
- A time zone for each account, a location from an IP address, and a link or a button in the notice.

These decisions apply:

- [ADR-0031](../adr/0031-flowspace-owns-authentication-and-revocable-sessions.md): Identity owns accounts and sessions.
- [ADR-0035](../adr/0035-identity-signup-security-and-mail-delivery.md): Identity queues each notice durably and sends it only to the verified email of the account.

## Contract

This capability adds no RPC, field, or event. It adds the value `new-device-notice` to the `identity_email_kind` label of the `identity_email_deliveries_total` metric in the [Observability metrics and dashboards](observability-metrics-and-dashboards.md) specification.

### Notification emails

| Part | Content |
| --- | --- |
| Recipient | The verified email of the account |
| Subject | `New sign-in to your Flowspace account` |
| Body | The login time in UTC, such as `17 Oct 2026, 13:38 UTC`, the device label, such as `Chrome on macOS`, and the sign-in method, such as `Password` or `Google`. Then a sentence that tells the owner to open Flowspace, review the sessions, and change the password when they do not recognize the sign-in |

## Behavior

### Known devices

- Identity computes the device fingerprint with a keyed hash of 3 values: the browser family and the operating-system family from the fixed user-agent parser of the [Identity session management](identity-session-management.md) specification, and the network of the trusted source IP address. The network is the first 24 bits of an IPv4 address or the first 48 bits of an IPv6 address.
- An unknown user agent gives `Unknown` for both families, and a missing trusted address gives an empty network. Each combination still gives 1 fingerprint.
- The hash key is an Identity secret, so a copy of the database alone cannot recover the network of a fingerprint.
- A known device holds the subject, the fingerprint, and its last-used time. It holds no IP address, user agent, or session ID.
- A known device expires 90 days after its last-used time. An expired known device does not count as known, and `identity-worker` deletes it within 24 hours after its expiry.

### Logins

- When `CreatePasswordSession` or `CreateProviderSession` creates a session, Identity computes the fingerprint of the request in the same transaction. This includes a restricted session under the [Identity account deletion](identity-account-deletion.md) specification.
- When the fingerprint matches an unexpired known device, Identity sets its last-used time to the login time and queues no notice.
- When the fingerprint matches no unexpired known device and the account has at least 1 unexpired known device, Identity adds the fingerprint as a known device and queues 1 new-device notice.
- When the account has no unexpired known device, Identity adds the fingerprint as a known device and queues no notice. This covers the first login after this capability starts and an account whose known devices all expired.
- `CreateAccount` and `ClaimUnverifiedAccount` add the fingerprint of their request as a known device without a notice.
- A successful `RefreshSession` sets the last-used time of a matching unexpired known device. It never adds a known device and never queues a notice.
- Re-authentication in an existing session neither changes the known devices nor queues a notice.

### Notices

- Identity queues the notice only when the account email is verified. For an unverified email, Identity still adds the known device.
- The notice commits with the session in 1 transaction. If delivery fails after the commit, Identity retries the notice without a change to the session.
- The login time in the notice is the commit time of the login. The device label is the label of the session device data, and the sign-in method is the method of the session.
- Each new fingerprint gives at most 1 notice, because the login adds it as known. For an account with a verified email and at least 1 unexpired known device, 2 logins from different new devices give 2 notices.
- Logins that run at the same time for 1 account run 1 at a time for the known devices. For the same account, 2 logins with the same new fingerprint give 1 notice.

### Security and abuse

- The existing login limits bound the number of notices. This capability adds no limit bucket.
- A person whose browser family, operating-system family, and network match an unexpired known device gets no notice. The maintainer accepted this limit.
- The notice contains no full IP address, location, link, code, or token, so a copy of the notice cannot lead the owner to a false page.

### Data and compatibility

- A migration adds a known-device table with a unique fingerprint for each subject, and a new-device notice kind to the durable notice queue.
- The final deletion and the account claim of the [Identity account deletion](identity-account-deletion.md) and [Identity signup and email verification](identity-signup-and-email-verification.md) specifications delete the known devices of the old subject in their transaction.
- A database failure, a timeout, or a cancellation before the commit leaves the session, the known devices, and the notice unchanged.
- Older code ignores the new table and notice kind. Before a rollback, delete the queued new-device notices. The known devices can stay, and a new version uses them again.

### Diagnostics

The mail worker counts each new-device notice delivery in `identity_email_deliveries_total` with the kind `new-device-notice`. Logs, traces, metrics, errors, and security events must never contain these values:

- Fingerprints, IP addresses, networks, user agents, full email addresses, and tokens.
- Session IDs and subject IDs in metric labels.

## Testing strategy

| Risk | Test level |
| --- | --- |
| The fingerprint uses the full IP address, the full user agent, or an untrusted header, or it changes for the same browser family, operating-system family, and network (`ID-T15`) | Unit |
| A long, malformed, or hostile user agent breaks the fingerprint (`ID-T15`, `ID-T16`) | Unit |
| A login from a known device queues a notice, or a login from a new device queues none | Integration |
| A login or a refresh from a known device does not set its last-used time, or a refresh changes another known device | Integration |
| A delivery failure after the commit loses the notice or changes the session, or a retry does not send the pending notice | Integration |
| The first known device of an account, a signup, or a claim queues a notice | Integration |
| A refresh or a re-authentication adds a known device or queues a notice | Integration |
| A known device counts as known after 90 days, or stays stored more than 24 hours after its expiry, before, at, and after the boundary (`ID-T19`, `ID-T21`) | Unit, Integration |
| A notice goes to an unverified email, or the known device is missing for an unverified email | Integration |
| Concurrent logins with the same new fingerprint queue more than 1 notice (`ID-T12`) | Integration |
| A notice contains an IP address, a location, a link, a code, or a token (`ID-T04`, `ID-T17`) | Integration |
| A database failure, a timeout, or a cancellation leaves a session without its known device or notice, or the opposite (`ID-T12`) | Integration |
| The final deletion or a claim leaves known devices of the old subject (`ID-T21`) | Integration |
| A database copy without the key recovers a network from a fingerprint (`ID-T18`) | Unit |
| Telemetry or errors contain a fingerprint, an IP address, a network, a user agent, or a token (`ID-T17`) | Integration |
| A new-device login fails end to end through the generated REST routes, the worker, and the mail delivery | Cluster |

## Implementation boundaries

Ask the maintainer before these actions:

- Change the 90-day expiry, the network sizes, or the values of the fingerprint.
- Store an IP address, a user agent, or a location in a known device.
- Add a link, a code, or a location to the notice.

## Success criteria

1. Given an account with a verified email and 1 known device, When the user signs in with a password from a new network, Then Identity queues 1 new-device notice and adds the fingerprint as known.
2. Given the same account, When the user signs in again from the same browser family, operating-system family, and network, Then Identity queues no notice and sets the last-used time of the known device.
3. Given an account with no unexpired known device, When the user signs in, Then Identity adds the fingerprint and queues no notice.
4. Given a new signup or a claimed account, When the session starts, Then Identity adds the fingerprint and queues no notice.
5. Given an account with a verified email, another unexpired known device, and a known device whose last-used time is more than 90 days old, When the user signs in from the old device, Then Identity queues 1 notice.
6. Given a session refreshes from a new network, When the refresh commits, Then Identity adds no known device and queues no notice.
7. Given an account with an unverified email, When the user signs in from a new device, Then Identity adds the known device and queues no notice.
8. Given an account in its deletion grace period with a verified email and an unexpired known device, When the user signs in from a new device, Then Identity queues 1 notice with the restricted session.
9. Given an account with a verified email and an unexpired known device, and 2 concurrent logins with the same new fingerprint, When both commit, Then Identity queues 1 notice.
10. Given a new-device notice, When the mail worker sends it, Then it contains the UTC login time, the device label, and the sign-in method. It contains no IP address, location, link, code, or token. The `identity_email_deliveries_total` metric increases for the kind `new-device-notice`.
11. Given a known device expired more than 24 hours ago, When `identity-worker` runs, Then the known device no longer exists.
12. Given an account reaches its final deletion or an unverified account is claimed, When the transaction commits, Then the old subject has no known device.
13. Given the new-device tests run, When a test inspects logs, traces, metrics, and errors, Then they contain no fingerprint, IP address, network, user agent, or token.
