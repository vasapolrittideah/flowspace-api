# ADR-0011: Pin each service to its own image digest in git

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `delivery`

## Context

The repository is one Go module holding several services (ADR-0009), and CI
builds only the services a commit actually touched. That is the difference
between a pipeline that finishes in two minutes and one that rebuilds eight
identical images every time a README changes.

It also breaks the obvious deployment scheme. If every manifest refers to
`image:$GIT_SHA`, then at any given commit most services have no image at that
tag, because they were not rebuilt. The deployment either fails to pull or,
worse, succeeds against something stale.

Mutable tags do not rescue this. With a moving `latest`, the deployment
controller cannot tell whether a sync changed anything, a rollback has no
address, and the node's image cache can serve yesterday's layers under today's
name — a failure that reproduces nowhere else.

Desired state lives in git. Image versions are part of desired state, so they
have to live there too.

## Decision

Each service is deployed by **digest**, recorded per service in the overlay for
its environment.

```yaml
# deploy/overlays/production/kustomization.yaml
images:
  - name: flowspace/workitem
    newName: ghcr.io/vasapol/flowspace-workitem
    digest: sha256:9f2c1e...
  - name: flowspace/identity
    newName: ghcr.io/vasapol/flowspace-identity
    digest: sha256:41ab7d...
```

A digest cannot be moved, so what is deployed is exactly what was built. There
is no `latest`, no branch tag, and no tag is ever pushed twice. Because a
digest is immutable by construction, `imagePullPolicy: IfNotPresent` is correct
and the node stops re-pulling images it already has.

Digests are unreadable, so the readable part is carried where humans look: the
commit that produced the image is a label on the workload and an annotation on
the pod template. **The tag is documentation; the digest is the contract.**

### Only what changed is updated

After pushing images for the services a commit rebuilt, CI updates the digest
for those services alone and commits the overlay. Every other service keeps the
digest it already had, which is still the digest of the last image built for
it. The overlay file is the record — there is nothing to compute and no state
outside git.

Two mechanical consequences follow and are part of the decision:

- CI commits to the repository that triggers CI, so the build workflow ignores
  changes under `deploy/`. Without that guard the pipeline triggers itself.
- Change detection must treat shared paths as a change to every service, for
  the reason already stated in ADR-0009. A missing entry in that filter
  produces a green pipeline and images that do not contain the change.

### Promotion moves the digest, never rebuilds

Promoting from staging to production copies the digest from one overlay to the
other. The artifact that passed its smoke run is the artifact that serves
traffic, byte for byte. Rebuilding for production would ship something no test
has ever seen, and would make the staging result evidence about a different
program.

### Unchanged services still have to be rebuilt

Building only what changed means a stable service is never rebuilt, and its
base image ages with every vulnerability published against it. A scheduled
weekly job rebuilds and re-pins **every** service regardless of changes. This
is not hygiene to add later; it is the cost of the build strategy and is
adopted with it.

## Consequences

### Positive

- Rollback is `git revert` of one commit, and it addresses an exact artifact.
- Every deployment is a reviewable diff, and the history of the overlay is the
  deployment history.
- Services that did not change are not redeployed, so unrelated work causes no
  pod churn.
- Digest pinning is the precondition for image signing and provenance
  attestation later, with no rework.

### Negative / accepted costs

- The repository accumulates commits written by CI rather than by a person.
  They are attributed to a bot and are noise in `git log` regardless.
- CI needs write access to the repository and a loop guard, which is a small
  amount of privilege pointed back at the thing that grants it.
- Nobody can read a digest. Any human answer to "what is running" comes from
  the label or the deployment UI, never from the manifest.
- The weekly rebuild redeploys everything, so it must be scheduled where a
  simultaneous restart of every service is acceptable.

### Neutral

- Staging and production differ by exactly one field per service, which makes
  the difference between environments inspectable rather than assumed.

## Alternatives considered

| Option                                                         | Why not                                                                                                                                                                      |
| -------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| One tag for the whole repository per commit                    | Requires building every service on every commit, which spends most of the CI budget producing images identical to the ones already in the registry                           |
| Mutable tags such as latest or a branch name                   | The deployment controller cannot tell whether anything changed, a rollback has no address to roll back to, and a node can serve stale layers under a name that looks current |
| Semantic version tags per service                              | Meaningful for a library with external consumers; here it is a second numbering scheme with no audience, maintained by hand alongside the one git already provides           |
| Rebuilding the image for production after staging              | Ships bytes that were never tested; promotion only means something when the artifact is unchanged                                                                            |
| A controller that watches the registry and updates deployments | Moves the source of truth out of git and into a controller reacting to a registry, which is the opposite of the reason for keeping desired state in git                      |

## Revisit when

- The volume of machine-written commits makes the repository history hard to
  read, which is the point at which deployment state moves to its own
  repository.
- A supply-chain requirement arrives — signed images, provenance, an admission
  policy that verifies them. Digest pinning is already the hard part.
- Full rebuilds become cheap enough that per-service tracking is complexity
  with nothing left to buy.

## References

- [Kubernetes — Images](https://kubernetes.io/docs/concepts/containers/images/)
- [Kustomize images transformer](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/images/)
- [OCI Image Specification](https://github.com/opencontainers/image-spec/blob/main/spec.md)
