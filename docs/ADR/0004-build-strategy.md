# ADR 0004: Build strategy — `docker build` shellout for prototype, Kaniko for production

**Status:** Accepted
**Date:** 2026-05-09

## Context

The submission service needs to turn a contestant's source archive into a runnable container image. Two competing constraints:

1. The hackathon demo is single-machine; a heavy build pipeline is overkill.
2. In production, contestant code is untrusted, so the build process itself must be sandboxed and must not have access to the host Docker socket.

## Decision

For the prototype, the submission service shells out to `docker build` and `docker push` against a local registry. The Docker daemon is the implicit BuildKit. This is fast, simple, and sufficient on a developer laptop or single-node cluster.

For production, swap the `BuildKit` builder implementation for a Kubernetes Job that runs Kaniko in a sandboxed pod with no access to the host's Docker socket. Same `Builder` interface, different implementation.

The `Builder` interface (`Enqueue(submissionID string)`) hides the choice; the rest of the service is unaware.

## Alternatives considered

- **Pure BuildKit standalone via gRPC API:** more features (caching, multi-platform), but operational complexity not justified at this stage. Modern Docker uses BuildKit under the hood anyway.
- **Buildah:** rootless build option. Less mature ecosystem on Kubernetes than Kaniko; rejected.
- **Pre-build everything as a single fat image:** rejected — kills the per-language toolchain isolation we want.

## Consequences

- Prototype requires the host running `submission-svc` to have access to a Docker daemon. The included docker-compose file provides a local registry on `localhost:5000`; the daemon must trust this insecure registry (Docker Desktop does so by default).
- The `Builder` interface is small (one method) so swapping to Kaniko is a single-file change plus a new K8s ServiceAccount.
- Build logs are routed through the service logger today; production should publish them to the existing object store under `submissions/<id>/build.log` so contestants can fetch them.

## Open follow-ups

- Build cache layer per language (saves 60s+ on Rust rebuilds).
- Fail builds whose Trivy scan returns HIGH/CRITICAL CVEs — wire into quality gate, not the build itself.
