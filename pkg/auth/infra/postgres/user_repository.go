package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/elug3/schick/pkg/auth/autherrors"
	"github.com/elug3/schick/pkg/auth/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// UserRepository implements ports.UserRepository using PostgreSQL.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new PostgreSQL user repository.
func NewUserRepository(ctx context.Context, db *sql.DB) (*UserRepository, error) {
	ur := &UserRepository{db: db}

	if err := ur.initTable(ctx); err != nil {
		return nil, err
	}

	return ur, nil
}

// FindByEmail finds a user by email.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, email, password, role, created_at FROM users WHERE email = $1", email)
	return r.scanUser(row)
}

// FindByID finds a user by ID.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, email, password, role, created_at FROM users WHERE id = $1", id.String())
	return r.scanUser(row)
}

// ListUsers returns all users ordered by creation time descending.
func (r *UserRepository) ListUsers(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, email, password, role, created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u, err := r.scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Save creates or updates a user. created_at is set only on insert.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	query := `INSERT INTO users (id, email, password, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email, password = EXCLUDED.password, role = EXCLUDED.role`
	_, err := r.db.ExecContext(ctx, query, user.ID.String(), user.Email, user.Password, user.Role)
	if isUniqueViolation(err) {
		return autherrors.ErrUserAlreadyExists
	}
	return err
}

// Delete deletes a user.
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", id.String())
	return err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func (r *UserRepository) scanUser(s scanner) (*domain.User, error) {
	var idStr string
	var user domain.User
	var createdAt time.Time
	err := s.Scan(&idStr, &user.Email, &user.Password, &user.Role, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	user.ID = id
	user.CreatedAt = createdAt.Format(time.RFC3339)
	return &user, nil
}

func (r *UserRepository) initTable(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS users (
		id         UUID PRIMARY KEY,
		email      TEXT UNIQUE NOT NULL,
		password   TEXT NOT NULL,
		role       TEXT NOT NULL DEFAULT 'user',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	if err != nil {
		return err
	}
	// Idempotent migrations for older schemas.
	for _, stmt := range []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user'`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
