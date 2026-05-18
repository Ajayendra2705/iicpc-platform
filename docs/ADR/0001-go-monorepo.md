# ADR 0001: Go monorepo with go.work

**Status:** Accepted
**Date:** 2026-05-09

## Context

Solo developer, 32-day window, 9 Go microservices needed (+ Next.js web).

## Decision

Use a single Go workspace (`go.work`) with one module per service under `services/`. All services share the `github.com/Ajayendra2705/iicpc-platform` namespace. Proto contracts live under `proto/` and generate into the same import path so every service imports types from the same place.

## Alternatives considered

- **Multi-repo:** rejected — overhead per repo (CI, versioning, PRs) too high for solo work.
- **Single Go module, multiple `cmd/`:** rejected — harder to enforce dependency boundaries between services.
- **Bazel:** rejected — onboarding cost not worth it for 9 services.

## Consequences

- Fast iteration: `go.work` lets local edits in one module propagate to consumers without `replace` directives.
- Single CI matrix.
- Risk: no enforced dependency boundary; rely on review discipline to keep services from importing each other directly. Use proto contracts as the only inter-service interface.
