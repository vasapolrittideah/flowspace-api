# Identity Bruno smoke tests

The collection creates disposable accounts against the local Identity API. It reads verification and claim codes from local Mailpit. It then tests password login, refresh-token replay, logout for one or all sessions, and password recovery. The recovery steps read the reset code and the password-change notice from Mailpit.

The provider login steps start Google and GitHub logins and check the authorization URLs. They also check that wrong-route, denied, and replayed callbacks and invalid handoff proofs issue no handoff code or session. These steps need both Google and GitHub client files in `.secrets/`, as the [README](../../../README.md) explains. They do not sign in to a real provider account.

Start these port forwards in separate terminals:

```sh
kubectl --context k3d-flowspace -n flowspace-local port-forward --address 127.0.0.1 svc/identity-api 18082:8080
kubectl --context k3d-flowspace -n flowspace-local port-forward --address 127.0.0.1 svc/mailpit-http 18025:8025
```

Run `task smoke:bruno` from the repository root. The collection waits 61 seconds before it requests a claim code because Identity limits code requests for one account. Each run sends three signup requests from one source address. Wait for the one-hour source limit to reset after repeated runs.
