package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	pkgotp "github.com/marees-godev/GoCart-Server/pkg/otp"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/pkg/redis"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/mailer"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/otp"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
	VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) (*dto.VerifyEmailResponse, error)
	ResendVerificationEmail(ctx context.Context, req *dto.ResendVerificationEmailRequest) (*dto.ResendVerificationEmailResponse, error)
}

type authService struct {
	repo           repository.AuthRepository
	cfg            *config.Config
	mailer         mailer.Mailer
	otpStore       otp.Store
	logger         *slog.Logger
	userClient     userpb.UserServiceClient
	merchantClient merchantpb.MerchantServiceClient
}

func NewAuthService(repo repository.AuthRepository, cfg *config.Config, log *slog.Logger, clients ...any) AuthService {
	if log == nil {
		log = slog.Default()
	}
	m := mailer.NewMailer(cfg.Email, log)
	return NewAuthServiceWithMailer(repo, cfg, m, log, clients...)
}

func NewAuthServiceWithMailer(repo repository.AuthRepository, cfg *config.Config, m mailer.Mailer, log *slog.Logger, clients ...any) AuthService {
	if log == nil {
		log = slog.Default()
	}
	s := &authService{
		repo:   repo,
		cfg:    cfg,
		mailer: m,
		logger: log,
	}
	for _, c := range clients {
		if c == nil {
			continue
		}
		switch client := c.(type) {
		case userpb.UserServiceClient:
			s.userClient = client
		case merchantpb.MerchantServiceClient:
			s.merchantClient = client
		case otp.Store:
			s.otpStore = client
		case *redis.Client:
			if client != nil {
				s.otpStore = otp.NewRedisStore(client)
			}
		}
	}
	if s.otpStore == nil {
		s.otpStore = otp.NewMemoryStore()
	}
	return s
}

func (s *authService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error) {

	if req == nil || strings.TrimSpace(req.Email) == "" || req.Password == "" {
		s.logger.Warn("Login attempt failed: missing email or password")
		return nil, appErrors.BadRequest("email and password are required")
	}

	email := cleanEmail(req.Email)
	role := model.RoleCustomer
	if req.IsMerchant {
		role = model.RoleMerchant
	}

	cred, err := s.repo.GetByEmailAndRole(ctx, email, role)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.Warn("Login attempt failed: user not found", "email", email, "role", role.String())
			return nil, appErrors.Unauthorized("Invalid email or password")
		}
		s.logger.Error("Login failed: database query error", "email", email, "error", err)
		return nil, appErrors.Internal(err, "failed to query credentials")
	}
	
	if !cred.EmailVerified {
		s.logger.Warn("Login attempt failed: email not verified", "user_id", cred.UserID.String(), "email", cred.Email)
		return nil, appErrors.Forbidden("email is not verified, please verify your email first and then login")
	}
	
	if cred.Role != "" && cred.Role != role {
		s.logger.Warn("Login attempt failed: role mismatch", "user_id", cred.UserID.String(), "email", cred.Email, "role", cred.Role.String(), "expected_role", role.String())
		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	if !cred.IsActive {
		s.logger.Warn("Login attempt failed: account inactive", "user_id", cred.UserID.String(), "email", cred.Email)
		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	if cred.LockedUntil != nil && time.Now().Before(*cred.LockedUntil) {
		s.logger.Warn("Login attempt failed: account locked", "user_id", cred.UserID.String(), "email", cred.Email, "locked_until", cred.LockedUntil)
		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(req.Password)); err != nil {
		failedCount := cred.FailedLoginCount + 1
		var lockedUntil *time.Time
		if failedCount >= MaxFailedLoginAttempts {
			t := time.Now().Add(AccountLockDuration)
			lockedUntil = &t
			s.logger.Warn("Account locked due to max failed login attempts", "user_id", cred.UserID.String(), "email", cred.Email, "failed_attempts", failedCount, "locked_until", t)
		} else {
			s.logger.Warn("Login attempt failed: invalid password", "user_id", cred.UserID.String(), "email", cred.Email, "failed_attempts", failedCount)
		}
		err = s.repo.UpdateFailedLogin(ctx, cred.ID, failedCount, lockedUntil)
		if err != nil {
			return nil, appErrors.Internal(err, "failed to update failed login")
		}
		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	if cred.FailedLoginCount > 0 {
		_ = s.repo.ResetFailedLogin(ctx, cred.ID)
	}


	var merchantID string
	var businessEmail string
	var firstName string
	var lastName string

	if role == model.RoleMerchant {
		mClient := s.merchantClient
		if mClient == nil && s.cfg != nil && s.cfg.Services.MerchantServiceURL != "" {
			var err error
			mClient, _, err = grpcclient.NewMerchantClient(s.cfg.Services.MerchantServiceURL, 5*time.Second)
			if err == nil {
				s.merchantClient = mClient
			}
		}

		if mClient != nil {
			mCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
				"x-user-id", cred.UserID.String(),
				"x-user-role", "MERCHANT",
			))
			mResp, err := mClient.GetMerchant(mCtx, &merchantpb.GetMerchantRequest{
				Id: cred.UserID.String(),
			})
			if err == nil && mResp != nil && mResp.Merchant != nil {
				merchantID = mResp.Merchant.Id
				businessEmail = mResp.Merchant.BusinessEmail
				firstName = mResp.Merchant.FirstName
				lastName = mResp.Merchant.LastName
			} else if err != nil {
				s.logger.Warn("Failed to fetch merchant details on login", "user_id", cred.UserID.String(), "error", err)
			}
		}
	}

	ttlMinutes := s.cfg.JWT.ExpiryMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 15
	}
	accessTTL := time.Duration(ttlMinutes) * time.Minute

	accessToken, err := auth.GenerateToken(auth.UserContext{
		UserID: cred.UserID.String(),
		Role:   cred.Role.String(),
		Email:  cred.Email,
	}, s.cfg.JWT.Secret, accessTTL)
	if err != nil {
		s.logger.Error("Login failed: access token generation error", "user_id", cred.UserID.String(), "error", err)
		return nil, appErrors.Internal(err, "failed to generate access token")
	}

	rawRefreshToken, err := generateRandomToken(32)
	if err != nil {
		s.logger.Error("Login failed: refresh token generation error", "user_id", cred.UserID.String(), "error", err)
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
		"role":         cred.Role.String(),
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
		AccessToken:   accessToken,
		RefreshToken:  rawRefreshToken,
		TokenType:     "Bearer",
		ExpiresIn:     int(accessTTL.Seconds()),
		UserID:        cred.UserID.String(),
		Role:          cred.Role.String(),
		MerchantID:    merchantID,
		BusinessEmail: businessEmail,
		FirstName:     firstName,
		LastName:      lastName,
	}, nil
}

func (s *authService) Register(ctx context.Context, req *dto.RegisterRequest) (*dto.LoginResponse, error) {
	if req == nil || strings.TrimSpace(req.Email) == "" || req.Password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	email := cleanEmail(req.Email)
	role := model.RoleCustomer
	if req.IsMerchant {
		role = model.RoleMerchant
	}

	existing, err := s.repo.GetByEmailAndRole(ctx, email, role)
	if err == nil && existing != nil {
		s.logger.Warn("Registration failed: user with email and role already exists", "email", email, "role", role.String())
		return nil, appErrors.Conflict("user with this email and role already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("Registration failed: password hash error", "email", email, "error", err)
		return nil, appErrors.Internal(err, "failed to hash password")
	}

	credID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())
	cred := &model.AuthCredential{
		ID:            credID,
		UserID:        userID,
		Email:         email,
		PasswordHash:  string(hashedPassword),
		Role:          role,
		EmailVerified: false,
		IsActive:      true,
	}

	if s.userClient == nil {
		s.logger.Error("Registration failed: user service client is not available")
		return nil, appErrors.ServiceUnavailable("user service unavailable")
	}

	if err := s.repo.CreateCredential(ctx, cred); err != nil {
		return nil, err
	}

	// Generate 6-digit verification OTP (valid for 5 minutes by default)
	rawOTP, err := generateOTP()
	if err == nil {
		ttlMinutes := s.cfg.Email.TokenTTLMinutes
		if ttlMinutes <= 0 {
			ttlMinutes = 5
		}
		otpHash := hashToken(rawOTP)
		ttl := time.Duration(ttlMinutes) * time.Minute
		if err := s.otpStore.SetOTP(ctx, email, otpHash, ttl); err == nil {
			go func(toEmail, otp string) {
				_ = s.mailer.SendVerificationEmail(context.Background(), toEmail, otp)
			}(email, rawOTP)
		} else {
			s.logger.Error("Failed to store verification OTP in store", "error", err)
		}
	}

	var merchantID string
	var businessEmail string
	if role == model.RoleMerchant {
		mClient := s.merchantClient
		if mClient == nil && s.cfg != nil && s.cfg.Services.MerchantServiceURL != "" {
			var err error
			mClient, _, err = grpcclient.NewMerchantClient(s.cfg.Services.MerchantServiceURL, 5*time.Second)
			if err != nil {
				s.logger.Error("Failed to connect to merchant service", "error", err)
				_ = s.repo.DeleteCredential(ctx, credID)
				return nil, appErrors.Internal(err, "failed to connect to merchant service")
			}
			s.merchantClient = mClient
		}

		if mClient != nil {
			mCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
				"x-user-id", userID.String(),
				"x-user-role", "MERCHANT",
			))

			createReq := &merchantpb.CreateMerchantRequest{
				Id:            userID.String(),
				FirstName:     req.FirstName,
				LastName:      req.LastName,
				BusinessEmail: req.Email,
			}

			mResp, err := mClient.CreateMerchant(mCtx, createReq)
			if err != nil {
				s.logger.Error("Failed to create merchant profile in merchant service", "user_id", userID.String(), "error", err)
				_ = s.repo.DeleteCredential(ctx, credID)
				return nil, appErrors.Internal(err, "failed to create merchant profile")
			}
			if mResp != nil && mResp.Merchant != nil {
				merchantID = mResp.Merchant.Id
				businessEmail = mResp.Merchant.BusinessEmail
			}
		}
	} else {
		_, err = s.userClient.CreateUser(ctx, &userpb.CreateUserRequest{
			Id:        userID.String(),
			Email:     cred.Email,
			FirstName: req.FirstName,
			LastName:  req.LastName,
		})
		if err != nil {
			_ = s.repo.DeleteCredential(ctx, credID)
			if st, ok := status.FromError(err); ok {
				switch st.Code() {
				case codes.AlreadyExists:
					return nil, appErrors.Conflict("user with this email already exists")
				case codes.InvalidArgument:
					return nil, appErrors.BadRequest(st.Message())
				}
			}
			return nil, appErrors.Internal(err, "failed to create user record in user service")
		}
	}
	ttlMinutes := s.cfg.JWT.ExpiryMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 15
	}
	accessTTL := time.Duration(ttlMinutes) * time.Minute

	accessToken, err := auth.GenerateToken(auth.UserContext{
		UserID: userID.String(),
		Role:   cred.Role.String(),
		Email:  cred.Email,
	}, s.cfg.JWT.Secret, accessTTL)
	if err != nil {
		s.logger.Error("Registration failed: generate token error", "user_id", userID.String(), "error", err)
		return nil, appErrors.Internal(err, "failed to generate access token")
	}

	rawRefreshToken, err := generateRandomToken(32)
	if err != nil {
		s.logger.Error("Registration failed: generate refresh token error", "user_id", userID.String(), "error", err)
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
		"role":          cred.Role.String(),
		"registered_at": time.Now().UTC(),
	})

	eventType := "UserRegistered"
	topic := "auth.user.registered"
	if role == model.RoleMerchant {
		eventType = "MerchantRegistered"
		topic = "auth.merchant.registered"
	}

	outboxEvt := &outbox.Event{
		ID:            uuid.Must(uuid.NewV7()),
		AggregateType: "auth",
		AggregateID:   userID.String(),
		EventType:     eventType,
		Topic:         topic,
		Payload:       eventPayload,
	}

	if err := s.repo.CreateLoginSession(ctx, refreshTokenModel, outboxEvt); err != nil {
		return nil, err
	}

	return &dto.LoginResponse{
		AccessToken:   accessToken,
		RefreshToken:  rawRefreshToken,
		TokenType:     "Bearer",
		ExpiresIn:     int(accessTTL.Seconds()),
		UserID:        userID.String(),
		Role:          cred.Role.String(),
		MerchantID:    merchantID,
		BusinessEmail: businessEmail,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
	}, nil
}

func (s *authService) ValidateToken(ctx context.Context, req *dto.ValidateTokenRequest) (*dto.ValidateTokenResponse, error) {

	if req == nil || req.Token == "" {
		s.logger.Warn("Token validation failed: missing token")
		return &dto.ValidateTokenResponse{Valid: false}, nil
	}

	userCtx, err := auth.ValidateToken(req.Token, s.cfg.JWT.Secret)
	if err != nil || userCtx == nil {
		s.logger.Warn("Token validation failed: invalid or expired token", "error", err)
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
		s.logger.Warn("Refresh token failed: missing refresh token")
		return nil, appErrors.BadRequest("refresh token is required")
	}

	tokenHash := hashToken(req.RefreshToken)
	tok, err := s.repo.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.Warn("Refresh token failed: token not found")
			return nil, appErrors.Unauthorized("invalid refresh token")
		}
		s.logger.Error("Refresh token failed: database error", "error", err)
		return nil, appErrors.Internal(err, "failed to query refresh token")
	}

	if tok.Revoked || time.Now().After(tok.ExpiresAt) {
		s.logger.Warn("Refresh token failed: token revoked or expired", "user_id", tok.UserID.String(), "revoked", tok.Revoked, "expires_at", tok.ExpiresAt)
		return nil, appErrors.Unauthorized("refresh token expired or revoked")
	}

	ttlMinutes := s.cfg.JWT.ExpiryMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 15
	}
	accessTTL := time.Duration(ttlMinutes) * time.Minute

	accessToken, err := auth.GenerateToken(auth.UserContext{
		UserID: tok.UserID.String(),
		Role:   model.RoleCustomer.String(),
	}, s.cfg.JWT.Secret, accessTTL)
	if err != nil {
		s.logger.Error("Refresh token failed: generate access token error", "user_id", tok.UserID.String(), "error", err)
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

func (s *authService) VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) (*dto.VerifyEmailResponse, error) {
	if req == nil || (req.OTP == "" && req.Token == "") {
		s.logger.Warn("Verify email failed: missing verification code")
		return nil, appErrors.BadRequest("verification code is required")
	}

	otpVal := strings.TrimSpace(req.OTP)
	if otpVal == "" {
		otpVal = strings.TrimSpace(req.Token)
	}

	email := cleanEmail(req.Email)
	if email == "" {
		s.logger.Warn("Verify email failed: missing email")
		return nil, appErrors.BadRequest("email is required")
	}

	cred, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.Warn("Verify email failed: user not found", "email", email)
			return nil, appErrors.NotFound("user not found")
		}
		s.logger.Error("Verify email failed: database query error", "error", err)
		return nil, appErrors.Internal(err, "failed to query user credentials")
	}

	if cred.EmailVerified {
		return &dto.VerifyEmailResponse{
			Success: true,
			Message: "Email address is already verified",
		}, nil
	}

	storedHash, err := s.otpStore.GetOTP(ctx, email)
	if err != nil {
		s.logger.Warn("Verify email failed: OTP expired or not found", "email", email)
		return nil, appErrors.BadRequest("verification code has expired or is invalid")
	}

	if storedHash != hashToken(otpVal) {
		s.logger.Warn("Verify email failed: code mismatch", "user_id", cred.UserID.String())
		return nil, appErrors.BadRequest("invalid verification code")
	}

	// Delete from Redis so OTP cannot be reused
	_ = s.otpStore.DeleteOTP(ctx, email)

	// Update user in PostgreSQL
	if err := s.repo.MarkEmailVerified(ctx, cred.UserID); err != nil {
		return nil, appErrors.Internal(err, "failed to update email verification status")
	}

	s.logger.Info("Email verified successfully with OTP", "user_id", cred.UserID.String(), "email", email)
	return &dto.VerifyEmailResponse{
		Success: true,
		Message: "Email address verified successfully",
	}, nil
}

func (s *authService) ResendVerificationEmail(ctx context.Context, req *dto.ResendVerificationEmailRequest) (*dto.ResendVerificationEmailResponse, error) {
	if req == nil || strings.TrimSpace(req.Email) == "" {
		s.logger.Warn("Resend verification email failed: missing email")
		return nil, appErrors.BadRequest("email is required")
	}

	email := cleanEmail(req.Email)
	cred, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.Warn("Resend verification email failed: user not found", "email", email)
			return nil, appErrors.NotFound("user not found")
		}
		return nil, appErrors.Internal(err, "failed to query user credentials")
	}

	if cred.EmailVerified {
		s.logger.Warn("Resend verification email failed: email already verified", "email", email)
		return nil, appErrors.BadRequest("email is already verified")
	}

	ttlMinutes := s.cfg.Email.TokenTTLMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 5
	}
	totalTTL := time.Duration(ttlMinutes) * time.Minute

	cooldownSec := s.cfg.Email.ResendCooldownSeconds
	if cooldownSec <= 0 {
		cooldownSec = 60
	}
	cooldown := time.Duration(cooldownSec) * time.Second

	// Enforce minimum cooldown period by checking remaining TTL in store before issuing a new OTP
	if remainingTTL, err := s.otpStore.GetTTL(ctx, email); err == nil && remainingTTL > 0 {
		timeElapsed := totalTTL - remainingTTL
		if timeElapsed < cooldown {
			waitSec := int((cooldown - timeElapsed + time.Second - 1) / time.Second)
			if waitSec < 1 {
				waitSec = 1
			}
			s.logger.Warn("Resend verification email rate limited: cooldown active",
				"email", email,
				"wait_seconds", waitSec,
			)
			return nil, appErrors.TooManyRequests(fmt.Sprintf("please wait %d seconds before requesting a new verification code", waitSec))
		}
	} else if err != nil && !errors.Is(err, otp.ErrOTPNotFound) {
		s.logger.Warn("Failed to check OTP remaining TTL, proceeding with resend", "error", err)
	}

	rawOTP, err := generateOTP()
	if err != nil {
		return nil, appErrors.Internal(err, "failed to generate verification OTP")
	}

	otpHash := hashToken(rawOTP)
	if err := s.otpStore.SetOTP(ctx, email, otpHash, totalTTL); err != nil {
		return nil, appErrors.Internal(err, "failed to store verification OTP in store")
	}

	go func(toEmail, otp string) {
		_ = s.mailer.SendVerificationEmail(context.Background(), toEmail, otp)
	}(email, rawOTP)

	return &dto.ResendVerificationEmailResponse{
		Success: true,
		Message: "Verification OTP sent successfully",
	}, nil
}

func cleanEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func generateOTP() (string, error) {
	return pkgotp.GenerateNumeric(6)
}

func generateRandomToken(nBytes int) (string, error) {
	return pkgotp.GenerateRandomToken(nBytes)
}

func hashToken(token string) string {
	return pkgotp.HashToken(token)
}

