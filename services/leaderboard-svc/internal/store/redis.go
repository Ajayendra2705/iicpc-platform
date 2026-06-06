package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

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
// leaderboard ranks each contestant by their best attempt. The HSET→HVALS→ZADD
// sequence is non-atomic but is only driven by the single-goroutine ingester,
// so concurrent interleaving for the same contestant does not occur.
func (r *Redis) UpsertSubmission(ctx context.Context, contestantID, submissionID string, score int64) error {
	if submissionID == "" {
		submissionID = "default"
	}
	subsKey := r.key + ":subs:" + contestantID
	if err := r.client.HSet(ctx, subsKey, submissionID, score).Err(); err != nil {
		return fmt.Errorf("hset %s %s=%d: %w", subsKey, submissionID, score, err)
	}
	vals, err := r.client.HVals(ctx, subsKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("hvals %s: %w", subsKey, err)
	}
	best := score
	for _, v := range vals {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil && n > best {
			best = n
		}
	}
	if err := r.client.ZAdd(ctx, r.key, redis.Z{Score: float64(best), Member: contestantID}).Err(); err != nil {
		return fmt.Errorf("zadd best %s=%d: %w", contestantID, best, err)
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
