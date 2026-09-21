package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

const (
	MaxFailedLoginAttempts = 5
	AccountLockDuration    = 15 * time.Minute
	RefreshTokenTTL        = 7 * 24 * time.Hour
)

type AuthService interface {
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error)
	Register(ctx context.Context, req *dto.RegisterRequest) (*dto.LoginResponse, error)
	ValidateToken(ctx context.Context, req *dto.ValidateTokenRequest) (*dto.ValidateTokenResponse, error)
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.LoginResponse, error)
}

type authService struct {
	repo repository.AuthRepository
	cfg  *config.Config
}

func NewAuthService(repo repository.AuthRepository, cfg *config.Config) AuthService {
	return &authService{
		repo: repo,
		cfg:  cfg,
	}
}

func (s *authService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error) {
	if req == nil || req.Email == "" || req.Password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	cred, err := s.repo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, appErrors.Unauthorized("Invalid email or password")
		}
		return nil, appErrors.Internal(err, "failed to query credentials")
	}

	if !cred.IsActive {
		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	if cred.LockedUntil != nil && time.Now().Before(*cred.LockedUntil) {
		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(req.Password)); err != nil {
		failedCount := cred.FailedLoginCount + 1
		var lockedUntil *time.Time
		if failedCount >= MaxFailedLoginAttempts {
			t := time.Now().Add(AccountLockDuration)
			lockedUntil = &t
		}
		_ = s.repo.UpdateFailedLogin(ctx, cred.ID, failedCount, lockedUntil)
		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	ttlMinutes := s.cfg.JWT.ExpiryMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 15
	}
	accessTTL := time.Duration(ttlMinutes) * time.Minute

	accessToken, err := auth.GenerateToken(auth.UserContext{
		UserID: cred.UserID.String(),
		Role:   cred.Role,
		Email:  cred.Email,
	}, s.cfg.JWT.Secret, accessTTL)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to generate access token")
	}

	rawRefreshToken, err := generateRandomToken(32)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to generate refresh token")
	}

	tokenHash := hashToken(rawRefreshToken)
	refreshTokenModel := &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    cred.UserID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(RefreshTokenTTL),
		Revoked:   false,
	}

	eventPayload, _ := json.Marshal(map[string]any{
		"user_id":      cred.UserID.String(),
		"email":        cred.Email,
		"role":         cred.Role,
		"logged_in_at": time.Now().UTC(),
	})

	outboxEvt := &outbox.Event{
		ID:            uuid.Must(uuid.NewV7()),
		AggregateType: "auth",
		AggregateID:   cred.UserID.String(),
		EventType:     "UserLoggedIn",
		Topic:         "auth.user.logged_in",
		Payload:       eventPayload,
	}

	if err := s.repo.CreateLoginSession(ctx, refreshTokenModel, outboxEvt); err != nil {
		return nil, appErrors.Internal(err, "failed to store login session")
	}

	return &dto.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTTL.Seconds()),
		UserID:       cred.UserID.String(),
	}, nil
}

func (s *authService) Register(ctx context.Context, req *dto.RegisterRequest) (*dto.LoginResponse, error) {
	if req == nil || req.Email == "" || req.Password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	existing, err := s.repo.GetByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		return nil, appErrors.Conflict("user with this email already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to hash password")
	}

	credID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())
	cred := &model.AuthCredential{
		ID:            credID,
		UserID:        userID,
		Email:         req.Email,
		PasswordHash:  string(hashedPassword),
		Role:          "CUSTOMER",
		EmailVerified: false,
		IsActive:      true,
	}

	if err := s.repo.CreateCredential(ctx, cred); err != nil {
		return nil, appErrors.Internal(err, "failed to create auth credential")
	}

	ttlMinutes := s.cfg.JWT.ExpiryMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 15
	}
	accessTTL := time.Duration(ttlMinutes) * time.Minute

	accessToken, err := auth.GenerateToken(auth.UserContext{
		UserID: userID.String(),
		Role:   cred.Role,
		Email:  cred.Email,
	}, s.cfg.JWT.Secret, accessTTL)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to generate access token")
	}

	rawRefreshToken, err := generateRandomToken(32)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to generate refresh token")
	}

	tokenHash := hashToken(rawRefreshToken)
	refreshTokenModel := &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(RefreshTokenTTL),
		Revoked:   false,
	}

	eventPayload, _ := json.Marshal(map[string]any{
		"user_id":       userID.String(),
		"email":         cred.Email,
		"role":          cred.Role,
		"registered_at": time.Now().UTC(),
	})

	outboxEvt := &outbox.Event{
		ID:            uuid.Must(uuid.NewV7()),
		AggregateType: "auth",
		AggregateID:   userID.String(),
		EventType:     "UserRegistered",
		Topic:         "auth.user.registered",
		Payload:       eventPayload,
	}

	if err := s.repo.CreateLoginSession(ctx, refreshTokenModel, outboxEvt); err != nil {
		return nil, appErrors.Internal(err, "failed to store registration session")
	}

	return &dto.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTTL.Seconds()),
		UserID:       userID.String(),
	}, nil
}

func (s *authService) ValidateToken(ctx context.Context, req *dto.ValidateTokenRequest) (*dto.ValidateTokenResponse, error) {
	if req == nil || req.Token == "" {
		return &dto.ValidateTokenResponse{Valid: false}, nil
	}

	userCtx, err := auth.ValidateToken(req.Token, s.cfg.JWT.Secret)
	if err != nil || userCtx == nil {
		return &dto.ValidateTokenResponse{Valid: false}, nil
	}

	return &dto.ValidateTokenResponse{
		Valid:  true,
		UserID: userCtx.UserID,
		Email:  userCtx.Email,
		Role:   userCtx.Role,
	}, nil
}

func (s *authService) RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.LoginResponse, error) {
	if req == nil || req.RefreshToken == "" {
		return nil, appErrors.BadRequest("refresh token is required")
	}

	tokenHash := hashToken(req.RefreshToken)
	tok, err := s.repo.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, appErrors.Unauthorized("invalid refresh token")
		}
		return nil, appErrors.Internal(err, "failed to query refresh token")
	}

	if tok.Revoked || time.Now().After(tok.ExpiresAt) {
		return nil, appErrors.Unauthorized("refresh token expired or revoked")
	}

	ttlMinutes := s.cfg.JWT.ExpiryMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 15
	}
	accessTTL := time.Duration(ttlMinutes) * time.Minute

	accessToken, err := auth.GenerateToken(auth.UserContext{
		UserID: tok.UserID.String(),
		Role:   "CUSTOMER",
	}, s.cfg.JWT.Secret, accessTTL)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to generate access token")
	}

	return &dto.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: req.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTTL.Seconds()),
		UserID:       tok.UserID.String(),
	}, nil
}

func generateRandomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
