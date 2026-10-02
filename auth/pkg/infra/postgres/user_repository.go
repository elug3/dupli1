package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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

// FindByEmail finds a user by email, case-insensitively — matches on
// LOWER(email) so callers don't need to normalize case themselves, and
// backs onto the ux_users_email_lower index created in migrateSchema.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `SELECT id, email, password, account_type, service_name, permissions, is_active, locked_at, failed_login_attempts, created_at
	          FROM users WHERE LOWER(email) = LOWER($1)`
	row := r.db.QueryRowContext(ctx, query, email)
	return scanUser(row)
}

// FindByID finds a user by ID.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	query := `SELECT id, email, password, account_type, service_name, permissions, is_active, locked_at, failed_login_attempts, created_at
	          FROM users WHERE id = $1`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanUser(row)
}

// Save creates or updates a user. Returns ErrUserAlreadyExists on email conflict.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	// created_at is set on insert only (the column default when the caller
	// left it nil) and never rewritten by an update.
	query := `INSERT INTO users (id, email, password, account_type, permissions, is_active, locked_at, failed_login_attempts, service_name, created_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE($10, NOW()))
	          ON CONFLICT (id) DO UPDATE
	            SET email = $2, password = $3, account_type = $4, permissions = $5,
	                is_active = $6, locked_at = $7, failed_login_attempts = $8, service_name = $9`
	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Email, user.Password, user.AccountType, pq.Array(user.Permissions),
		user.IsActive, user.LockedAt, user.FailedLoginAttempts, user.ServiceName, user.CreatedAt,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return autherrors.ErrUserAlreadyExists
		}
		return fmt.Errorf("save: %w", err)
	}
	return nil
}

// Delete removes a user by ID, and its API keys with it.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete: begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM service_api_keys WHERE user_id = $1", id); err != nil {
		return fmt.Errorf("delete api keys: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete: commit: %w", err)
	}
	return nil
}

// ListAll returns all users ordered by email.
func (r *UserRepository) ListAll(ctx context.Context) ([]*domain.User, error) {
	query := `SELECT id, email, password, account_type, service_name, permissions, is_active, locked_at, failed_login_attempts, created_at
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
	var lockedAt, createdAt sql.NullTime
	err := s.Scan(
		&u.ID, &u.Email, &u.Password, &u.AccountType, &u.ServiceName, pq.Array(&u.Permissions),
		&u.IsActive, &lockedAt, &u.FailedLoginAttempts, &createdAt,
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
	if createdAt.Valid {
		u.CreatedAt = &createdAt.Time
	}
	return &u, nil
}

// RegistrationTimes returns when each accountType account in [start, end)
// was created, and how many such accounts have no recorded creation time.
func (r *UserRepository) RegistrationTimes(ctx context.Context, accountType string, start, end time.Time) ([]time.Time, int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT created_at FROM users WHERE account_type = $1 AND created_at >= $2 AND created_at < $3`,
		accountType, start, end)
	if err != nil {
		return nil, 0, fmt.Errorf("registration times: %w", err)
	}
	defer rows.Close()
	times := make([]time.Time, 0)
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return nil, 0, fmt.Errorf("registration times: %w", err)
		}
		times = append(times, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("registration times: rows: %w", err)
	}

	var undated int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE account_type = $1 AND created_at IS NULL`, accountType,
	).Scan(&undated); err != nil {
		return nil, 0, fmt.Errorf("registration times: undated: %w", err)
	}
	return times, undated, nil
}
