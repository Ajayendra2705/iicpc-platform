-- Telemetry storage schema for the aggregator service.
-- Apply against a fresh database that has the TimescaleDB extension installed.
--
--   psql "$POSTGRES_DSN" -f 001_telemetry_schema.sql
--
-- Requires Timescale ≥ 2.10 for `continuous_aggregate_policy` and
-- `retention_policy` on continuous aggregates.

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ----------------------------------------------------------------------------
-- Raw 1-second window snapshots emitted by the aggregator on every Flush().
-- One row per (contestant_id, submission_id, window_start). submission_id
-- identifies the attempt; it is '' for legacy single-run telemetry that
-- carries no submission. Including it in the key keeps a contestant's distinct
-- attempts as distinct rows so per-submission analytics stay exact.
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS telemetry_snapshots (
    contestant_id TEXT             NOT NULL,
    submission_id TEXT             NOT NULL DEFAULT '',
    window_start  TIMESTAMPTZ      NOT NULL,
    duration_ns   BIGINT           NOT NULL,
    count         BIGINT           NOT NULL,
    rejected      BIGINT           NOT NULL,
    timeouts      BIGINT           NOT NULL,
    tps           DOUBLE PRECISION NOT NULL,
    p50_ns        BIGINT           NOT NULL,
    p90_ns        BIGINT           NOT NULL,
    p99_ns        BIGINT           NOT NULL,
    p999_ns       BIGINT           NOT NULL,
    PRIMARY KEY (contestant_id, submission_id, window_start)
);

SELECT create_hypertable(
    'telemetry_snapshots',
    'window_start',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists       => TRUE
);

CREATE INDEX IF NOT EXISTS idx_telemetry_snapshots_contestant_time
    ON telemetry_snapshots (contestant_id, submission_id, window_start DESC);

-- ----------------------------------------------------------------------------
-- 1-minute continuous aggregate. Powers leaderboard time-series queries.
--
-- IMPORTANT CORRECTNESS NOTE: averaging P99 across 60 one-second windows is
-- statistically WRONG — you cannot average percentiles. A contestant with
-- P99=1ms in window A and P99=100ms in window B has merged P99 closer to
-- 100ms than to 50.5ms.
--
-- This view's avg_p99_ns / avg_p50_ns columns therefore systematically
-- *understate* tail latency and must be treated as approximations.
-- max_p99_ns is exact per bucket and should be preferred for monitoring.
--
-- The correct fix is to store the per-window HDR histogram bytes and merge
-- bucket-by-bucket. The aggregator service exposes this exactly via:
--
--     GET /metrics/merged/{contestant_id}?windows=60
--
-- which merges the in-memory ring of recent histograms. To do the same in
-- SQL would require a TimescaleDB user-defined aggregate over a BYTEA
-- column holding hdrhistogram-encoded snapshots — deferred to a future
-- migration since the in-memory merge already covers the leaderboard path.
-- ----------------------------------------------------------------------------
CREATE MATERIALIZED VIEW IF NOT EXISTS telemetry_1m
WITH (timescaledb.continuous) AS
SELECT
    contestant_id,
    submission_id,
    time_bucket('1 minute', window_start) AS bucket,
    SUM(count)        AS total_count,
    SUM(rejected)     AS total_rejected,
    SUM(timeouts)     AS total_timeouts,
    AVG(tps)          AS avg_tps,
    AVG(p50_ns)::BIGINT AS avg_p50_ns,
    AVG(p99_ns)::BIGINT AS avg_p99_ns,
    MAX(p99_ns)       AS max_p99_ns,
    MAX(p999_ns)      AS max_p999_ns
FROM telemetry_snapshots
GROUP BY contestant_id, submission_id, bucket
WITH NO DATA;

-- Refresh every 30s, materialising buckets that closed ≥ 30s ago.
SELECT add_continuous_aggregate_policy(
    'telemetry_1m',
    start_offset      => INTERVAL '10 minutes',
    end_offset        => INTERVAL '30 seconds',
    schedule_interval => INTERVAL '30 seconds',
    if_not_exists     => TRUE
);

-- Retention: 7 days raw, 30 days 1m rollups.
SELECT add_retention_policy('telemetry_snapshots', INTERVAL '7 days',  if_not_exists => TRUE);
SELECT add_retention_policy('telemetry_1m',        INTERVAL '30 days', if_not_exists => TRUE);
