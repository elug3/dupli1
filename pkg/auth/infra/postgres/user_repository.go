package postgres

import (
	"context"
	"database/sql"
	"errors"

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
	query := "SELECT id, email, password FROM users WHERE email = $1"
	row := r.db.QueryRowContext(ctx, query, email)

	var idStr string
	var user domain.User
	if err := row.Scan(&idStr, &user.Email, &user.Password); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	user.ID = id

	return &user, nil
}

// FindByID finds a user by ID.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := "SELECT id, email, password FROM users WHERE id = $1"
	row := r.db.QueryRowContext(ctx, query, id.String())

	var idStr string
	var user domain.User
	if err := row.Scan(&idStr, &user.Email, &user.Password); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	parsed, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	user.ID = parsed

	return &user, nil
}

// Save saves a user.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	query := "INSERT INTO users (id, email, password) VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET email = $2, password = $3"
	_, err := r.db.ExecContext(ctx, query, user.ID.String(), user.Email, user.Password)
	if isUniqueViolation(err) {
		return autherrors.ErrUserAlreadyExists
	}
	return err
}

// Delete deletes a user.
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := "DELETE FROM users WHERE id = $1"
	_, err := r.db.ExecContext(ctx, query, id.String())
	return err
}

func (r *UserRepository) initTable(ctx context.Context) error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY,
		email TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL
	)
	`)
	if err != nil {
		return err
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
