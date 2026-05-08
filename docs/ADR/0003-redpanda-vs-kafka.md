# ADR 0003: Redpanda over Kafka for the event bus

**Status:** Accepted
**Date:** 2026-05-09

## Context

Need a Kafka-compatible event bus for telemetry events, score updates, and audit logs. Scale target: 100K+ events/sec sustained.

## Decision

Use Redpanda. Single binary, no ZooKeeper, Kafka-API-compatible.

## Alternatives considered

- **Apache Kafka (MSK):** mature, battle-tested. Drawback: ZooKeeper or KRaft setup overhead, JVM tuning, more nodes to operate.
- **NATS JetStream:** simpler but the ecosystem of consumers (kafka-go, librdkafka) is smaller; we want the option to swap to MSK in production.
- **Cloud-native (Kinesis / Pub/Sub):** vendor lock-in; harder to run locally.

## Consequences

- Local dev: `docker run redpandadata/redpanda` is one container.
- Cloud: Redpanda Cloud or self-hosted on EKS. MSK Serverless remains a fallback if Redpanda Cloud cost is prohibitive — wire format is compatible.
