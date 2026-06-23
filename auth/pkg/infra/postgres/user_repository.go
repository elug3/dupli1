package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/elug3/schick/auth/pkg/domain"
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
	query := "SELECT id, email, password FROM users WHERE email = $1"
	row := r.db.QueryRowContext(ctx, query, email)

	var user domain.User
	err := row.Scan(&user.ID, &user.Email, &user.Password)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find by email: %w", err)
	}

	return &user, nil
}

// FindByID finds a user by ID.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	query := "SELECT id, email, password FROM users WHERE id = $1"
	row := r.db.QueryRowContext(ctx, query, id)

	var user domain.User
	err := row.Scan(&user.ID, &user.Email, &user.Password)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}

	return &user, nil
}

// Save saves a user.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	query := "INSERT INTO users (id, email, password) VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET email = $2, password = $3"
	_, err := r.db.ExecContext(ctx, query, user.ID, user.Email, user.Password)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	return nil
}

// Delete deletes a user.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	query := "DELETE FROM users WHERE id = $1"
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}
