package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// upsertSubmissionScript records one submission's score in the per-contestant
// hash (KEYS[1]) and sets the contestant's ZSET (KEYS[2]) score to the max
// across their submissions, all in one atomic server-side step. Running it as a
// single EVAL is what makes the HSET→max→ZADD safe when several leaderboard-svc
// replicas ingest concurrently — without it the read-modify-write would race and
// leave a stale best in the ZSET. ARGV: member, submission, score.
var upsertSubmissionScript = redis.NewScript(`
local subsKey = KEYS[1]
local zsetKey = KEYS[2]
local member = ARGV[1]
local submission = ARGV[2]
local score = tonumber(ARGV[3])
redis.call('HSET', subsKey, submission, score)
local best = score
for _, v in ipairs(redis.call('HVALS', subsKey)) do
  local n = tonumber(v)
  if n and n > best then best = n end
end
redis.call('ZADD', zsetKey, best, member)
return best
`)

// RedisConfig configures the ZSET-backed store.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	Key      string // ZSET key, e.g. "leaderboard:scores"
}

// Redis persists scores in a Redis ZSET ordered by score.
type Redis struct {
	client *redis.Client
	key    string
}

func NewRedis(cfg RedisConfig) *Redis {
	if cfg.Key == "" {
		cfg.Key = "leaderboard:scores"
	}
	return &Redis{
		client: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
		key: cfg.Key,
	}
}

// Ping verifies the connection; call from main on startup.
func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *Redis) Upsert(ctx context.Context, id string, score int64) error {
	if err := r.client.ZAdd(ctx, r.key, redis.Z{Score: float64(score), Member: id}).Err(); err != nil {
		return fmt.Errorf("zadd %s=%d: %w", id, score, err)
	}
	return nil
}

// UpsertSubmission records one submission's score in a per-contestant hash and
// sets the contestant's ZSET score to the max across their submissions, so the
// leaderboard ranks each contestant by their best attempt. The HSET→max→ZADD is
// executed as one atomic Lua script, so it stays correct even when several
// leaderboard-svc replicas ingest the same contestant concurrently.
func (r *Redis) UpsertSubmission(ctx context.Context, contestantID, submissionID string, score int64) error {
	if submissionID == "" {
		submissionID = "default"
	}
	subsKey := r.key + ":subs:" + contestantID
	keys := []string{subsKey, r.key}
	if err := upsertSubmissionScript.Run(ctx, r.client, keys, contestantID, submissionID, score).Err(); err != nil {
		return fmt.Errorf("upsert submission %s/%s=%d: %w", contestantID, submissionID, score, err)
	}
	return nil
}

func (r *Redis) Top(ctx context.Context, n int) ([]Entry, error) {
	if n <= 0 {
		n = 100
	}
	zs, err := r.client.ZRevRangeWithScores(ctx, r.key, 0, int64(n-1)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("zrevrange: %w", err)
	}
	out := make([]Entry, 0, len(zs))
	for _, z := range zs {
		out = append(out, Entry{ContestantID: z.Member.(string), Score: int64(z.Score)})
	}
	return out, nil
}

func (r *Redis) Close() error { return r.client.Close() }

var _ Store = (*Redis)(nil)
