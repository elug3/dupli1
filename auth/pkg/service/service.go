package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
)

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

const (
	userRegisteredSubject = "user.registered"
	maxFailedAttempts     = 5
)

// Service holds all auth business logic.
type Service struct {
	userRepo           ports.UserRepository
	tokenGen           ports.TokenGenerator // issues short-lived access tokens
	refreshTokenGen    ports.TokenGenerator // issues long-lived refresh tokens
	sessionStore       ports.SessionStore   // persists active refresh tokens; nil = revocation disabled
	refreshTokenExpiry time.Duration
	eventPublisher     ports.EventPublisher
}

// ServiceOption configures a Service.
type ServiceOption func(*Service)

// WithRefreshTokenGen sets the token generator and expiry used for refresh tokens.
func WithRefreshTokenGen(gen ports.TokenGenerator, expiry time.Duration) ServiceOption {
	return func(s *Service) {
		s.refreshTokenGen = gen
		s.refreshTokenExpiry = expiry
	}
}

// WithSessionStore enables refresh-token revocation via a persistent store.
func WithSessionStore(store ports.SessionStore) ServiceOption {
	return func(s *Service) {
		s.sessionStore = store
	}
}

// WithEventPublisher sets the integration-event publisher.
func WithEventPublisher(pub ports.EventPublisher) ServiceOption {
	return func(s *Service) {
		s.eventPublisher = pub
	}
}

// NewService creates a new auth Service.
// tokenGen issues short-lived access tokens; use WithRefreshTokenGen to set a
// separate long-lived generator. If omitted, the same generator is used for both.
func NewService(userRepo ports.UserRepository, tokenGen ports.TokenGenerator, opts ...ServiceOption) *Service {
	s := &Service{userRepo: userRepo, tokenGen: tokenGen}
	for _, o := range opts {
		o(s)
	}
	if s.refreshTokenGen == nil {
		s.refreshTokenGen = tokenGen
	}
	return s
}

type userRegisteredEvent struct {
	EventType   string    `json:"event_type"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	AccountType string    `json:"account_type"`
	Occurred    time.Time `json:"occurred_at"`
}

// Register creates a new user. Roles defaults to ["customer"] when empty.
// accountType defaults to customer when empty.
func (s *Service) Register(ctx context.Context, email, password, accountType string, roles ...string) (*domain.User, error) {
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		return nil, autherrors.ErrInvalidEmail
	}
	if len(password) < 8 {
		return nil, autherrors.ErrWeakPassword
	}
	if accountType == "" {
		accountType = domain.DefaultAccountType
	}
	if !domain.ValidAccountType(accountType) {
		return nil, autherrors.ErrInvalidAccountType
	}
	if len(roles) == 0 {
		roles = []string{"customer"}
	}
	u, err := domain.NewUser(newID(), email, password, accountType, roles...)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if err := s.userRepo.Save(ctx, u); err != nil {
		return nil, fmt.Errorf("save user: %w", err)
	}
	if err := s.publishUserRegistered(ctx, u); err != nil {
		_ = s.userRepo.Delete(ctx, u.ID)
		return nil, fmt.Errorf("publish event: %w", err)
	}
	return u, nil
}

// Login validates credentials, tracks failed attempts, and returns a refresh token.
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

	if !u.IsActive {
		return "", autherrors.ErrAccountDeactivated
	}

	if !u.ValidatePassword(password) {
		u.FailedLoginAttempts++
		if u.FailedLoginAttempts >= maxFailedAttempts {
			u.Lock()
		}
		_ = s.userRepo.Save(ctx, u)
		return "", autherrors.ErrInvalidCredentials
	}

	if u.FailedLoginAttempts > 0 {
		u.FailedLoginAttempts = 0
		_ = s.userRepo.Save(ctx, u)
	}

	token, err := s.refreshTokenGen.Generate(ctx, u.ID, u.Roles)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Set(ctx, token, u.ID, s.refreshTokenExpiry); err != nil {
			return "", fmt.Errorf("store session: %w", err)
		}
	}

	return token, nil
}

// Logout revokes a refresh token from the session store.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if s.sessionStore == nil {
		return nil
	}
	return s.sessionStore.Delete(ctx, refreshToken)
}

// Refresh validates a refresh token and issues a new short-lived access token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (string, error) {
	claims, err := s.refreshTokenGen.Validate(ctx, refreshToken)
	if err != nil {
		return "", err
	}

	if s.sessionStore != nil {
		if _, err := s.sessionStore.Get(ctx, refreshToken); err != nil {
			if errors.Is(err, ports.ErrSessionNotFound) {
				return "", autherrors.ErrInvalidToken
			}
			return "", fmt.Errorf("session store: %w", err)
		}
	}

	u, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return "", fmt.Errorf("find user: %w", err)
	}
	if u == nil {
		return "", autherrors.ErrUserNotFound
	}
	if !u.IsActive {
		return "", autherrors.ErrAccountDeactivated
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
		return nil, err // ErrTokenExpired or ErrInvalidToken from tokenGen
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

// SetUserRole replaces the role list for the given user.
// When accountType is non-empty it also updates User.AccountType.
func (s *Service) SetUserRole(ctx context.Context, userID string, roles []string, accountType string) (*domain.User, error) {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if u == nil {
		return nil, autherrors.ErrUserNotFound
	}
	if accountType != "" {
		if !domain.ValidAccountType(accountType) {
			return nil, autherrors.ErrInvalidAccountType
		}
		u.AccountType = accountType
	}
	u.SetRoles(roles)
	if err := s.userRepo.Save(ctx, u); err != nil {
		return nil, fmt.Errorf("save user: %w", err)
	}
	return u, nil
}

// UpdateUserPassword hashes newPassword and persists it for the given user.
func (s *Service) UpdateUserPassword(ctx context.Context, userID, newPassword string) error {
	if len(newPassword) < 8 {
		return autherrors.ErrWeakPassword
	}
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if u == nil {
		return autherrors.ErrUserNotFound
	}
	if err := u.UpdatePassword(newPassword); err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.userRepo.Save(ctx, u)
}

// SetUserStatus sets the active/inactive status for the given user.
func (s *Service) SetUserStatus(ctx context.Context, userID string, isActive bool) (*domain.User, error) {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if u == nil {
		return nil, autherrors.ErrUserNotFound
	}
	u.SetActive(isActive)
	if err := s.userRepo.Save(ctx, u); err != nil {
		return nil, fmt.Errorf("save user: %w", err)
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
		EventType:   userRegisteredSubject,
		UserID:      u.ID,
		Email:       u.Email,
		AccountType: u.AccountType,
		Occurred:    time.Now().UTC(),
	})
}
