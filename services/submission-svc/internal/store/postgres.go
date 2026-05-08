package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Embedded migration. Idempotent; runs on every startup.
const migrationSQL = `
CREATE TABLE IF NOT EXISTS submissions (
    id            TEXT PRIMARY KEY,
    contestant_id TEXT NOT NULL,
    language      TEXT NOT NULL,
    status        TEXT NOT NULL,
    source_uri    TEXT NOT NULL,
    image_uri     TEXT,
    entrypoint    TEXT NOT NULL,
    build_error   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_submissions_contestant_created ON submissions (contestant_id, created_at DESC);
`

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := pool.Exec(ctx, migrationSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) Create(s Submission) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := p.pool.Exec(ctx, `
		INSERT INTO submissions
		    (id, contestant_id, language, status, source_uri, image_uri, entrypoint, build_error, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,NULLIF($8,''),$9,$10)
	`, s.ID, s.ContestantID, string(s.Language), string(s.Status),
		s.SourceURI, s.ImageURI, s.Entrypoint, s.BuildError, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		// Duplicate primary key surfaces as a unique-violation; map to ErrAlreadyExists.
		if isUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (p *Postgres) Get(id string) (Submission, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := p.pool.QueryRow(ctx, `
		SELECT id, contestant_id, language, status, source_uri,
		       COALESCE(image_uri,''), entrypoint, COALESCE(build_error,''),
		       created_at, updated_at
		FROM submissions WHERE id = $1
	`, id)
	var s Submission
	var lang, status string
	if err := row.Scan(&s.ID, &s.ContestantID, &lang, &status, &s.SourceURI,
		&s.ImageURI, &s.Entrypoint, &s.BuildError, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Submission{}, false
		}
		return Submission{}, false
	}
	s.Language = Language(lang)
	s.Status = Status(status)
	return s, true
}

func (p *Postgres) UpdateStatus(id string, status Status, imageURI, buildError string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tag, err := p.pool.Exec(ctx, `
		UPDATE submissions
		   SET status = $2,
		       image_uri = COALESCE(NULLIF($3,''), image_uri),
		       build_error = NULLIF($4,''),
		       updated_at = NOW()
		 WHERE id = $1
	`, id, string(status), imageURI, buildError)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) List(contestantID string, limit int) []Submission {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := []any{}
	q := `SELECT id, contestant_id, language, status, source_uri,
		       COALESCE(image_uri,''), entrypoint, COALESCE(build_error,''),
		       created_at, updated_at FROM submissions`
	if contestantID != "" {
		q += ` WHERE contestant_id = $1`
		args = append(args, contestantID)
	}
	q += ` ORDER BY created_at DESC`
	if limit > 0 {
		if contestantID != "" {
			q += ` LIMIT $2`
		} else {
			q += ` LIMIT $1`
		}
		args = append(args, limit)
	}
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []Submission
	for rows.Next() {
		var s Submission
		var lang, status string
		if err := rows.Scan(&s.ID, &s.ContestantID, &lang, &status, &s.SourceURI,
			&s.ImageURI, &s.Entrypoint, &s.BuildError, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		s.Language = Language(lang)
		s.Status = Status(status)
		out = append(out, s)
	}
	return out
}

func isUniqueViolation(err error) bool {
	// pgx returns *pgconn.PgError with Code "23505" for unique violations.
	type sqlState interface{ SQLState() string }
	var s sqlState
	if errors.As(err, &s) && s.SQLState() == "23505" {
		return true
	}
	return false
}
