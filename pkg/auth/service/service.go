package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/schick/pkg/auth/autherrors"
	"github.com/elug3/schick/pkg/auth/domain"
	"github.com/elug3/schick/pkg/auth/ports"
	"github.com/google/uuid"
)

type Service struct {
	userRepo           ports.UserRepository
	accessTokenGen     ports.TokenGenerator
	refreshTokenGen    ports.TokenGenerator
	sessionStore       ports.SessionStore
	refreshTokenExpiry time.Duration
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// NewService creates a new auth Service with required dependencies.
func NewService(
	userRepo ports.UserRepository,
	accessTokenGen ports.TokenGenerator,
	refreshTokenGen ports.TokenGenerator,
	sessionStore ports.SessionStore,
	refreshTokenExpiry time.Duration,
) *Service {
	return &Service{
		userRepo:           userRepo,
		accessTokenGen:     accessTokenGen,
		refreshTokenGen:    refreshTokenGen,
		sessionStore:       sessionStore,
		refreshTokenExpiry: refreshTokenExpiry,
	}
}

// Register creates a new user (minimal signature).
func (s *Service) Register(ctx context.Context, email, password string) (*domain.User, error) {
	u := domain.NewUser(email)
	if err := u.SetPassword(password); err != nil {
		return nil, err
	}

	if err := s.userRepo.Save(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Login validates credentials and returns access and refresh tokens.
func (s *Service) Login(ctx context.Context, email, password string) (TokenPair, error) {
	u, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return TokenPair{}, err
	}
	if u == nil {
		return TokenPair{}, autherrors.ErrInvalidCredentials
	}

	if !u.ValidatePassword(password) {
		return TokenPair{}, autherrors.ErrInvalidCredentials
	}

	tokens, err := s.issueTokenPair(ctx, u.ID)
	if err != nil {
		return TokenPair{}, err
	}

	return tokens, nil
}

// Logout handles logout logic (e.g., invalidate session). Signature accepts context.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.refreshTokenGen.Validate(ctx, refreshToken, ports.TokenTypeRefresh)
	if err != nil {
		return autherrors.ErrInvalidToken
	}
	if claims.SessionID == "" {
		return autherrors.ErrInvalidToken
	}

	storedUserID, err := s.sessionStore.Get(ctx, refreshSessionKey(claims.SessionID))
	if err != nil {
		return autherrors.ErrInvalidToken
	}
	if storedUserID != claims.UserID.String() {
		return autherrors.ErrInvalidToken
	}

	return s.sessionStore.Delete(ctx, refreshSessionKey(claims.SessionID))
}

// Me validates a token and returns the matching user.
func (s *Service) Me(ctx context.Context, token string) (*domain.User, error) {
	claims, err := s.accessTokenGen.Validate(ctx, token, ports.TokenTypeAccess)
	if err != nil {
		return nil, autherrors.ErrInvalidToken
	}

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, autherrors.ErrUserNotFound
	}

	return user, nil
}

// Refresh validates a refresh token and issues a new token pair.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	claims, err := s.refreshTokenGen.Validate(ctx, refreshToken, ports.TokenTypeRefresh)
	if err != nil {
		return TokenPair{}, autherrors.ErrInvalidToken
	}
	if claims.SessionID == "" {
		return TokenPair{}, autherrors.ErrInvalidToken
	}

	storedUserID, err := s.sessionStore.Get(ctx, refreshSessionKey(claims.SessionID))
	if err != nil {
		return TokenPair{}, autherrors.ErrInvalidToken
	}
	if storedUserID != claims.UserID.String() {
		return TokenPair{}, autherrors.ErrInvalidToken
	}

	if err := s.sessionStore.Delete(ctx, refreshSessionKey(claims.SessionID)); err != nil {
		return TokenPair{}, err
	}

	return s.issueTokenPair(ctx, claims.UserID)
}

func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID) (TokenPair, error) {
	if s.sessionStore == nil {
		return TokenPair{}, errors.New("session store is required")
	}

	sessionID := uuid.NewString()
	if err := s.sessionStore.Set(ctx, refreshSessionKey(sessionID), userID.String(), s.refreshTokenExpiry); err != nil {
		return TokenPair{}, err
	}

	accessToken, err := s.accessTokenGen.Generate(ctx, userID, ports.TokenTypeAccess, "")
	if err != nil {
		_ = s.sessionStore.Delete(ctx, refreshSessionKey(sessionID))
		return TokenPair{}, err
	}

	refreshToken, err := s.refreshTokenGen.Generate(ctx, userID, ports.TokenTypeRefresh, sessionID)
	if err != nil {
		_ = s.sessionStore.Delete(ctx, refreshSessionKey(sessionID))
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func refreshSessionKey(sessionID string) string {
	return fmt.Sprintf("auth:refresh:%s", sessionID)
}
