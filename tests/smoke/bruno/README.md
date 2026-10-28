# Identity Bruno smoke tests

The collection creates disposable accounts against the local Identity API. It reads verification and claim codes from local Mailpit. It then tests password login, refresh-token replay, logout for one or all sessions, and password recovery. The recovery steps read the reset code and the password-change notice from Mailpit.

The provider login steps start Google and GitHub logins and make sure that the authorization URLs are correct. They also make sure that wrong-route, denied, and replayed callbacks and invalid handoff proofs issue no handoff code or session. These steps need both Google and GitHub client files in `.secrets/`, as the [Flowspace API](../../../README.md) README explains. They do not sign in to a real provider account.

The re-authentication steps log in again and re-authenticate the new session with its password and with a code from Mailpit. They make sure that a missing token, a wrong password, and a wrong or used code return no end time. They also make sure that the code keeps the end time of the open window. They start a GitHub re-authentication and make sure that the start rejects an `Idempotency-Key` and that a malformed handoff code fails. They do not sign in to GitHub.

Start these port forwards in separate terminals:

```sh
kubectl --context k3d-flowspace -n flowspace-local port-forward --address 127.0.0.1 svc/identity-api 18082:8080
kubectl --context k3d-flowspace -n flowspace-local port-forward --address 127.0.0.1 svc/mailpit-http 18025:8025
```

Run `task smoke:bruno` from the repository root. Identity limits code requests for 1 account. The collection waits 61 seconds before it requests a claim code and before it requests a re-authentication code. Each run sends 3 signup requests from 1 source address. After repeated runs, wait for the source limit of 1 hour to reset.
