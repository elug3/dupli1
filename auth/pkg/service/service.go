package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/elug3/schick/auth/pkg/autherrors"
	"github.com/elug3/schick/auth/pkg/domain"
	"github.com/elug3/schick/auth/pkg/ports"
)

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

const userRegisteredSubject = "user.registered"

type Service struct {
	userRepo       ports.UserRepository
	tokenGen       ports.TokenGenerator
	eventPublisher ports.EventPublisher
}

type userRegisteredEvent struct {
	EventType string    `json:"event_type"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Occurred  time.Time `json:"occurred_at"`
}

// NewService creates a new auth Service with required dependencies.
func NewService(userRepo ports.UserRepository, tokenGen ports.TokenGenerator, eventPublisher ...ports.EventPublisher) *Service {
	s := &Service{userRepo: userRepo, tokenGen: tokenGen}
	if len(eventPublisher) > 0 {
		s.eventPublisher = eventPublisher[0]
	}
	return s
}

// Register creates a new user. Roles defaults to ["customer"] when empty.
func (s *Service) Register(ctx context.Context, email, password string, roles ...string) (*domain.User, error) {
	if len(roles) == 0 {
		roles = []string{"customer"}
	}
	u, err := domain.NewUser(newID(), email, password, roles...)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if err := s.userRepo.Save(ctx, u); err != nil {
		return nil, fmt.Errorf("save user: %w", err)
	}
	if err := s.publishUserRegistered(ctx, u); err != nil {
		return nil, fmt.Errorf("publish event: %w", err)
	}
	return u, nil
}

// Login validates credentials and returns a token.
func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	u, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return "", fmt.Errorf("find user: %w", err)
	}
	if u == nil {
		return "", autherrors.ErrInvalidCredentials
	}

	if u.IsLocked() {
		return "", autherrors.ErrAccountLocked
	}

	if !u.ValidatePassword(password) {
		return "", autherrors.ErrInvalidCredentials
	}

	token, err := s.tokenGen.Generate(ctx, u.ID, u.Roles)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	return token, nil
}

// Logout handles logout logic (e.g., invalidate session).
func (s *Service) Logout(ctx context.Context, userID string) error {
	// TODO: implement session invalidation
	_ = userID
	return nil
}

// Refresh validates a token and issues a new one with fresh roles from the DB.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (string, error) {
	claims, err := s.tokenGen.Validate(ctx, refreshToken)
	if err != nil {
		return "", fmt.Errorf("validate token: %w", err)
	}

	u, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return "", fmt.Errorf("find user: %w", err)
	}
	if u == nil {
		return "", autherrors.ErrUserNotFound
	}

	newToken, err := s.tokenGen.Generate(ctx, u.ID, u.Roles)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	return newToken, nil
}

// GetMe validates an access token and returns the authenticated user.
func (s *Service) GetMe(ctx context.Context, accessToken string) (*domain.User, error) {
	claims, err := s.tokenGen.Validate(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("validate token: %w", autherrors.ErrInvalidToken)
	}

	u, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if u == nil {
		return nil, autherrors.ErrUserNotFound
	}

	return u, nil
}

// ListUsers returns all users. The caller is responsible for authorization.
func (s *Service) ListUsers(ctx context.Context) ([]*domain.User, error) {
	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

func (s *Service) publishUserRegistered(ctx context.Context, u *domain.User) error {
	if s.eventPublisher == nil {
		return nil
	}

	return s.eventPublisher.Publish(ctx, userRegisteredSubject, userRegisteredEvent{
		EventType: userRegisteredSubject,
		UserID:    u.ID,
		Email:     u.Email,
		Occurred:  time.Now().UTC(),
	})
}
