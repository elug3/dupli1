package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/lib/pq"
)

// UserRepository implements ports.UserRepository using PostgreSQL.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new PostgreSQL user repository.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByEmail finds a user by email.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `SELECT id, email, password, account_type, roles, is_active, locked_at, failed_login_attempts
	          FROM users WHERE email = $1`
	row := r.db.QueryRowContext(ctx, query, email)
	return scanUser(row)
}

// FindByID finds a user by ID.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	query := `SELECT id, email, password, account_type, roles, is_active, locked_at, failed_login_attempts
	          FROM users WHERE id = $1`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanUser(row)
}

// Save creates or updates a user. Returns ErrUserAlreadyExists on email conflict.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	query := `INSERT INTO users (id, email, password, account_type, roles, is_active, locked_at, failed_login_attempts)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	          ON CONFLICT (id) DO UPDATE
	            SET email = $2, password = $3, account_type = $4, roles = $5,
	                is_active = $6, locked_at = $7, failed_login_attempts = $8`
	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Email, user.Password, user.AccountType, pq.Array(user.Roles),
		user.IsActive, user.LockedAt, user.FailedLoginAttempts,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return autherrors.ErrUserAlreadyExists
		}
		return fmt.Errorf("save: %w", err)
	}
	return nil
}

// Delete removes a user by ID.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

// ListAll returns all users ordered by email.
func (r *UserRepository) ListAll(ctx context.Context) ([]*domain.User, error) {
	query := `SELECT id, email, password, account_type, roles, is_active, locked_at, failed_login_attempts
	          FROM users ORDER BY email`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list all: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("list all: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list all: rows: %w", err)
	}
	return users, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (*domain.User, error) {
	var u domain.User
	var lockedAt sql.NullTime
	err := s.Scan(
		&u.ID, &u.Email, &u.Password, &u.AccountType, pq.Array(&u.Roles),
		&u.IsActive, &lockedAt, &u.FailedLoginAttempts,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	if lockedAt.Valid {
		u.LockedAt = &lockedAt.Time
	}
	return &u, nil
}
