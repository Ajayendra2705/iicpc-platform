# ADR 0002: gVisor for contestant pod isolation

**Status:** Accepted
**Date:** 2026-05-09

## Context

Contestants upload arbitrary code (C++, Rust, Go) that we must run on shared infrastructure. We need strong isolation between submissions and the host, with low overhead.

## Decision

Use gVisor (`runsc`) as the container runtime for all contestant pods, set via `runtimeClassName: gvisor` on the PodSpec. Combine with cgroup v2 limits, dropped capabilities, read-only root, and a restrictive NetworkPolicy.

## Alternatives considered

- **Firecracker / Kata Containers:** stronger isolation (full VM) but slower cold start (seconds vs ~300ms) and more operational complexity. Overkill for the demo.
- **Plain runc + seccomp:** weaker isolation; a kernel CVE in a syscall the seccomp profile allows could escape. Insufficient for arbitrary code.
- **Docker without K8s:** loses native scheduling and resource governance.

## Consequences

- Some syscalls have measurable overhead under gVisor; latency-sensitive contestant code may report slightly higher numbers than on bare runc. This is acceptable because all contestants run under identical conditions, so rankings remain fair.
- Local kind clusters do not include gVisor by default; local development falls back to runc with strict seccomp. Production EKS nodes will install `runsc`.
