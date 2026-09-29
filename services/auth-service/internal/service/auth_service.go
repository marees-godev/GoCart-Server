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
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/validator"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	MaxFailedLoginAttempts = 5
	AccountLockDuration    = 15 * time.Minute
	RefreshTokenTTL        = 7 * 24 * time.Hour
	dummyBcryptHash        = "$2a$10$e8N7z.0O2w9tP/V1O8m1o.7Hq6G3V3HkX/N1.1.1.1.1.1.1.1"
)

type AuthService interface {
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error)
	Register(ctx context.Context, req *dto.RegisterRequest) (*dto.LoginResponse, error)
	ValidateToken(ctx context.Context, req *dto.ValidateTokenRequest) (*dto.ValidateTokenResponse, error)
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.LoginResponse, error)
	Logout(ctx context.Context, req *dto.LogoutRequest) (*dto.LogoutResponse, error)
	VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) (*dto.VerifyEmailResponse, error)
	ResendVerificationEmail(ctx context.Context, req *dto.ResendVerificationEmailRequest) (*dto.ResendVerificationEmailResponse, error)
	ForgotPassword(ctx context.Context, req *dto.ForgotPasswordRequest) (*dto.ForgotPasswordResponse, error)
	ResetPasswordWithOtp(ctx context.Context, req *dto.ResetPasswordWithOtpRequest) (*dto.ResetPasswordResponse, error)
	ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) (*dto.ChangePasswordResponse, error)
}

type authService struct {
	repo           repository.AuthRepository
	cfg            *config.Config
	mailer         mailer.Mailer
	otpStore       otp.Store
	resetStore     otp.PasswordResetStore
	redisClient    *redis.Client
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
		case otp.PasswordResetStore:
			s.resetStore = client
		case *redis.Client:
			if client != nil {
				s.redisClient = client
				s.otpStore = otp.NewRedisStore(client)
				s.resetStore = otp.NewRedisPasswordResetStore(client)
			}
		}
	}
	if s.otpStore == nil {
		s.otpStore = otp.NewMemoryStore()
	}
	if s.resetStore == nil {
		s.resetStore = otp.NewMemoryPasswordResetStore()
	}
	return s
}

func failedLoginKey(email string) string {
	return "auth:login:failed:" + email
}

func accountLockKey(email string) string {
	return "auth:login:locked:" + email
}

func (s *authService) getMaxLoginAttempts() int {
	if s.cfg != nil && s.cfg.Security.MaxLoginAttempts > 0 {
		return s.cfg.Security.MaxLoginAttempts
	}
	return MaxFailedLoginAttempts
}

func (s *authService) getLoginAttemptWindow() time.Duration {
	if s.cfg != nil && s.cfg.Security.LoginAttemptWindow > 0 {
		return s.cfg.Security.LoginAttemptWindow
	}
	return 15 * time.Minute
}

func (s *authService) getAccountLockDuration() time.Duration {
	if s.cfg != nil && s.cfg.Security.AccountLockDuration > 0 {
		return s.cfg.Security.AccountLockDuration
	}
	return AccountLockDuration
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

	maxAttempts := s.getMaxLoginAttempts()
	window := s.getLoginAttemptWindow()
	lockDuration := s.getAccountLockDuration()

	failedKey := failedLoginKey(email)
	lockKey := accountLockKey(email)

	// 1. Check if account is locked in Redis
	if s.redisClient != nil {
		isLocked, err := s.redisClient.Exists(ctx, lockKey)
		if err != nil {
			s.logger.Warn("Redis error checking lock status, falling back to DB check", "email", email, "error", err)
		} else if isLocked {
			s.logger.Warn("Login attempt failed: account locked (Redis)", "email", email)
			return nil, appErrors.Unauthorized("Invalid email or password")
		}
	}

	// 2. Query user credential from DB
	cred, err := s.repo.GetByEmailAndRole(ctx, email, role)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.Warn("Login attempt failed: user not found", "email", email, "role", role.String())
			// Perform dummy hash comparison to prevent timing attack enumeration
			_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(req.Password))

			// Track failed attempt in Redis for normalized email even if user doesn't exist
			if s.redisClient != nil {
				count, rErr := s.redisClient.IncrWithExpiry(ctx, failedKey, window)
				if rErr != nil {
					s.logger.Warn("Redis error incrementing failed login counter", "email", email, "error", rErr)
				} else if int(count) >= maxAttempts {
					if lErr := s.redisClient.Set(ctx, lockKey, "locked", lockDuration); lErr != nil {
						s.logger.Warn("Redis error setting lock key", "email", email, "error", lErr)
					}
				}
			}

			return nil, appErrors.Unauthorized("Invalid email or password")
		}
		s.logger.Error("Login failed: database query error", "email", email, "error", err)
		return nil, appErrors.Internal(err, "failed to query credentials")
	}

	// 3. Check if account is locked in DB
	if cred.LockedUntil != nil && time.Now().Before(*cred.LockedUntil) {
		s.logger.Warn("Login attempt failed: account locked (DB)", "user_id", cred.UserID.String(), "email", cred.Email, "locked_until", cred.LockedUntil)
		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	// 4. Validate user status and role
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

	// 5. Compare Password
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(req.Password)); err != nil {
		var redisCount int64
		var redisErr error
		if s.redisClient != nil {
			redisCount, redisErr = s.redisClient.IncrWithExpiry(ctx, failedKey, window)
			if redisErr != nil {
				s.logger.Warn("Redis error incrementing failed attempts", "email", email, "error", redisErr)
			}
		}

		failedCount := cred.FailedLoginCount + 1
		if s.redisClient != nil && redisErr == nil {
			failedCount = int(redisCount)
		}

		var lockedUntil *time.Time
		if failedCount >= maxAttempts {
			t := time.Now().Add(lockDuration)
			lockedUntil = &t
			s.logger.Warn("Account locked due to max failed login attempts", "user_id", cred.UserID.String(), "email", cred.Email, "failed_attempts", failedCount, "locked_until", t)
			if s.redisClient != nil {
				if lErr := s.redisClient.Set(ctx, lockKey, "locked", lockDuration); lErr != nil {
					s.logger.Warn("Redis error setting lock key", "email", email, "error", lErr)
				}
			}
		} else {
			s.logger.Warn("Login attempt failed: invalid password", "user_id", cred.UserID.String(), "email", cred.Email, "failed_attempts", failedCount)
		}

		// Always update DB as fallback/persistence
		dbErr := s.repo.UpdateFailedLogin(ctx, cred.ID, failedCount, lockedUntil)
		if dbErr != nil {
			s.logger.Error("Failed to update DB failed login record", "user_id", cred.UserID.String(), "error", dbErr)
		}

		return nil, appErrors.Unauthorized("Invalid email or password")
	}

	// 6. Successful Login: Reset failed login attempt counters and locks in Redis and DB
	if s.redisClient != nil {
		if rErr := s.redisClient.Delete(ctx, failedKey, lockKey); rErr != nil {
			s.logger.Warn("Redis error clearing failed login key on successful login", "email", email, "error", rErr)
		}
	}

	if cred.FailedLoginCount > 0 || cred.LockedUntil != nil {
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
			mCtx := auth.WithUser(ctx, &auth.UserContext{
				UserID: cred.UserID.String(),
				Role:   model.RoleMerchant.String(),
				Email:  cred.Email,
			})
			mCtx = metadata.NewOutgoingContext(mCtx, metadata.Pairs(
				"x-user-id", cred.UserID.String(),
				"x-user-role", model.RoleMerchant.String(),
			))
			mResp, err := mClient.GetMerchantByUserID(mCtx, &merchantpb.GetMerchantByUserIDRequest{
				UserId: cred.UserID.String(),
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
			businessName := strings.TrimSpace(req.BusinessName)
			if businessName == "" {
				businessName = strings.TrimSpace(req.FirstName + " " + req.LastName)
			}
			if businessName == "" {
				businessName = req.Email
			}

			mCtx := auth.WithUser(ctx, &auth.UserContext{
				UserID: userID.String(),
				Role:   model.RoleMerchant.String(),
				Email:  req.Email,
			})
			mCtx = metadata.NewOutgoingContext(mCtx, metadata.Pairs(
				"x-user-id", userID.String(),
				"x-user-role", model.RoleMerchant.String(),
			))

			createReq := &merchantpb.CreateMerchantRequest{
				UserId:        userID.String(),
				FirstName:     req.FirstName,
				LastName:      req.LastName,
				BusinessEmail: req.Email,
				BusinessName:  businessName,
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

	if userCtx.UserID != "" {
		if parsedID, err := uuid.FromString(userCtx.UserID); err == nil {
			if cred, err := s.repo.GetByUserID(ctx, parsedID); err == nil && cred != nil {
				if !cred.IsActive {
					return &dto.ValidateTokenResponse{Valid: false}, nil
				}
			}
		}
	}

	return &dto.ValidateTokenResponse{
		Valid:  true,
		UserID: userCtx.UserID,
		Email:  userCtx.Email,
		Role:   userCtx.Role,
	}, nil
}

func (s *authService) RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.LoginResponse, error) {
	if req == nil || strings.TrimSpace(req.RefreshToken) == "" {
		s.logger.Warn("Refresh token failed: missing refresh token")
		return nil, appErrors.BadRequest("refresh token is required")
	}

	var identity string
	if req.AccessToken != "" {
		if userCtx, err := auth.ExtractClaimsWithoutExpiry(req.AccessToken, s.cfg.JWT.Secret); err == nil && userCtx != nil {
			if userCtx.UserID != "" {
				identity = userCtx.UserID
			} else if userCtx.Email != "" {
				identity = userCtx.Email
			}
		}
	}

	tokenHash := hashToken(req.RefreshToken)
	tok, err := s.repo.GetRefreshToken(ctx, tokenHash, identity)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.Warn("Refresh token failed: token not found")
			return nil, appErrors.Unauthorized("invalid refresh token")
		}
		s.logger.Error("Refresh token failed: database error", "error", err)
		return nil, appErrors.Internal(err, "failed to query refresh token")
	}

	if tok.Revoked {
		s.logger.Warn("Refresh token failed: token is revoked", "user_id", tok.UserID.String())
		return nil, appErrors.Unauthorized("refresh token revoked")
	}

	if time.Now().After(tok.ExpiresAt) {
		s.logger.Warn("Refresh token failed: token is expired", "user_id", tok.UserID.String(), "expires_at", tok.ExpiresAt)
		return nil, appErrors.Unauthorized("refresh token expired")
	}

	cred, err := s.repo.GetByUserID(ctx, tok.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.Warn("Refresh token failed: associated user not found", "user_id", tok.UserID.String())
			return nil, appErrors.Unauthorized("associated user not found")
		}
		s.logger.Error("Refresh token failed: failed to get user", "user_id", tok.UserID.String(), "error", err)
		return nil, appErrors.Internal(err, "failed to query user credentials")
	}

	if !cred.IsActive {
		s.logger.Warn("Refresh token failed: user is deactivated", "user_id", tok.UserID.String())
		return nil, appErrors.Unauthorized("user account is deactivated")
	}

	ttlMinutes := s.cfg.JWT.ExpiryMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 15
	}
	accessTTL := time.Duration(ttlMinutes) * time.Minute

	accessToken, err := auth.GenerateToken(auth.UserContext{
		UserID: cred.UserID.String(),
		Email:  cred.Email,
		Role:   cred.Role.String(),
	}, s.cfg.JWT.Secret, accessTTL)
	if err != nil {
		s.logger.Error("Refresh token failed: generate access token error", "user_id", cred.UserID.String(), "error", err)
		return nil, appErrors.Internal(err, "failed to generate access token")
	}

	newRawRefreshToken, err := generateRandomToken(32)
	if err != nil {
		s.logger.Error("Refresh token failed: generate new refresh token error", "user_id", cred.UserID.String(), "error", err)
		return nil, appErrors.Internal(err, "failed to generate new refresh token")
	}

	newTokenHash := hashToken(newRawRefreshToken)
	newRefreshTokenModel := &model.RefreshToken{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    cred.UserID,
		TokenHash: newTokenHash,
		ExpiresAt: time.Now().Add(RefreshTokenTTL),
		Revoked:   false,
	}

	if err := s.repo.RotateRefreshToken(ctx, tok.ID, newRefreshTokenModel); err != nil {
		s.logger.Error("Refresh token failed: rotate token error", "user_id", cred.UserID.String(), "error", err)
		return nil, appErrors.Internal(err, "failed to rotate refresh token")
	}

	return &dto.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRawRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTTL.Seconds()),
		UserID:       cred.UserID.String(),
		Role:         cred.Role.String(),
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

func (s *authService) Logout(ctx context.Context, req *dto.LogoutRequest) (*dto.LogoutResponse, error) {
	if req == nil {
		req = &dto.LogoutRequest{}
	}

	var userIDStr string
	var emailStr string

	if req.AccessToken != "" {
		if userCtx, err := auth.ExtractClaimsWithoutExpiry(req.AccessToken, s.cfg.JWT.Secret); err == nil && userCtx != nil {
			if userCtx.UserID != "" {
				userIDStr = userCtx.UserID
			}
			if userCtx.Email != "" {
				emailStr = userCtx.Email
			}
		} else {
			userIDStr = req.AccessToken
		}
	}

	if userIDStr == "" && emailStr == "" {
		if userCtx, ok := auth.FromContext(ctx); ok && userCtx != nil {
			if userCtx.UserID != "" {
				userIDStr = userCtx.UserID
			}
			if userCtx.Email != "" {
				emailStr = userCtx.Email
			}
		}
	}

	var targetUserID uuid.UUID
	if userIDStr != "" {
		if parsed, err := uuid.FromString(userIDStr); err == nil {
			targetUserID = parsed
		}
	}

	if targetUserID == uuid.Nil && emailStr != "" {
		if cred, err := s.repo.GetByEmail(ctx, cleanEmail(emailStr)); err == nil && cred != nil {
			targetUserID = cred.UserID
		}
	}

	if targetUserID == uuid.Nil {
		s.logger.Warn("Logout failed: missing authentication context or identity")
		return nil, appErrors.Unauthorized("authentication required for logout")
	}

	if err := s.repo.RevokeRefreshTokensByUserID(ctx, targetUserID); err != nil {
		s.logger.Error("Logout failed: failed to revoke user refresh tokens", "user_id", targetUserID.String(), "error", err)
		return nil, appErrors.Internal(err, "failed to revoke refresh tokens")
	}

	s.logger.Info("Logout successful", "user_id", targetUserID.String())
	return &dto.LogoutResponse{
		Success: true,
		Message: "Logged out successfully",
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

func hashWithSalt(token, salt string) string {
	return pkgotp.HashToken(salt + ":" + token)
}

const uniformForgotPasswordMessage = "If an account exists with this email address, a password reset code has been sent."

func (s *authService) ForgotPassword(ctx context.Context, req *dto.ForgotPasswordRequest) (*dto.ForgotPasswordResponse, error) {
	if req == nil || strings.TrimSpace(req.Email) == "" {
		return nil, appErrors.BadRequest("email is required")
	}

	email := cleanEmail(req.Email)
	clientIP := strings.TrimSpace(req.ClientIP)

	maxReqs := 3
	if s.cfg != nil && s.cfg.Security.PasswordResetMaxRequests > 0 {
		maxReqs = s.cfg.Security.PasswordResetMaxRequests
	}
	window := 30 * time.Minute
	if s.cfg != nil && s.cfg.Security.PasswordResetRequestWindow > 0 {
		window = s.cfg.Security.PasswordResetRequestWindow
	}

	// 1. Rate limiting by email (Max 3 per 30 minutes)
	allowedEmail, err := s.resetStore.RecordRequest(ctx, email, maxReqs, window)
	if err == nil && !allowedEmail {
		s.logger.Warn("Forgot password rate limit exceeded by email", "email", email)
		return nil, appErrors.TooManyRequests("too many password reset requests; please try again later")
	}

	// 2. Rate limiting by IP (if provided)
	if clientIP != "" {
		allowedIP, err := s.resetStore.RecordRequest(ctx, "ip:"+clientIP, maxReqs, window)
		if err == nil && !allowedIP {
			s.logger.Warn("Forgot password rate limit exceeded by IP", "ip", clientIP)
			return nil, appErrors.TooManyRequests("too many password reset requests; please try again later")
		}
	}

	// 3. Uniform response / Enumeration Defense: Check if user exists
	cred, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.Info("Forgot password requested for non-existent email", "email", email)
			return &dto.ForgotPasswordResponse{
				Success: true,
				Message: uniformForgotPasswordMessage,
			}, nil
		}
		s.logger.Error("Forgot password database error", "email", email, "error", err)
		return nil, appErrors.Internal(err, "failed to query user credentials")
	}

	if !cred.IsActive {
		s.logger.Warn("Forgot password requested for inactive account", "email", email)
		return &dto.ForgotPasswordResponse{
			Success: true,
			Message: uniformForgotPasswordMessage,
		}, nil
	}

	// 4. Generate cryptographically secure 6-digit numeric OTP
	rawOTP, err := generateOTP()
	if err != nil {
		s.logger.Error("Failed to generate password reset OTP", "error", err)
		return nil, appErrors.Internal(err, "failed to generate password reset code")
	}

	salt, err := generateRandomToken(16)
	if err != nil {
		salt = "gocart-salt"
	}

	otpHash := hashWithSalt(rawOTP, salt)

	ttlMinutes := 15
	if s.cfg != nil && s.cfg.Security.PasswordResetOTPTTLMinutes > 0 {
		ttlMinutes = s.cfg.Security.PasswordResetOTPTTLMinutes
	}
	ttl := time.Duration(ttlMinutes) * time.Minute

	if err := s.resetStore.SetResetOTP(ctx, email, otpHash, salt, ttl); err != nil {
		s.logger.Error("Failed to store password reset OTP", "error", err)
		return nil, appErrors.Internal(err, "failed to store password reset code")
	}

	// 5. Send password reset email asynchronously
	go func(toEmail, otpCode string) {
		_ = s.mailer.SendPasswordResetEmail(context.Background(), toEmail, otpCode)
	}(email, rawOTP)

	s.logger.Info("Password reset OTP generated and email dispatched", "email", email)
	return &dto.ForgotPasswordResponse{
		Success: true,
		Message: uniformForgotPasswordMessage,
	}, nil
}

func (s *authService) ResetPasswordWithOtp(ctx context.Context, req *dto.ResetPasswordWithOtpRequest) (*dto.ResetPasswordResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be empty")
	}

	email := cleanEmail(req.Email)
	if email == "" {
		return nil, appErrors.BadRequest("email is required")
	}
	otpVal := strings.TrimSpace(req.OTP)
	if otpVal == "" {
		return nil, appErrors.BadRequest("otp is required")
	}
	if req.NewPassword == "" {
		return nil, appErrors.BadRequest("new password is required")
	}

	// 1. Validate password complexity first
	if err := validator.ValidatePasswordStrength(req.NewPassword); err != nil {
		return nil, appErrors.BadRequest(err.Error())
	}

	// 2. Check if locked out due to >= 5 failed attempts
	if isLocked, _, err := s.resetStore.IsLocked(ctx, email); err == nil && isLocked {
		s.logger.Warn("Reset password attempt blocked: lockout active", "email", email)
		return nil, appErrors.TooManyRequests("maximum invalid OTP attempts exceeded; password reset locked for 1 hour")
	}

	// 3. Retrieve stored reset OTP
	otpData, err := s.resetStore.GetResetOTP(ctx, email)
	if err != nil {
		s.logger.Warn("Reset password failed: OTP not found or expired", "email", email)
		return nil, appErrors.BadRequest("password reset code has expired or is invalid")
	}

	// 4. Verify salted hash of OTP
	expectedHash := hashWithSalt(otpVal, otpData.Salt)
	if otpData.Hash != expectedHash {
		attempts, _ := s.resetStore.IncrementAttempts(ctx, email)
		maxAttempts := 5
		if s.cfg != nil && s.cfg.Security.PasswordResetMaxAttempts > 0 {
			maxAttempts = s.cfg.Security.PasswordResetMaxAttempts
		}
		lockoutDuration := 1 * time.Hour
		if s.cfg != nil && s.cfg.Security.PasswordResetLockoutDuration > 0 {
			lockoutDuration = s.cfg.Security.PasswordResetLockoutDuration
		}

		if attempts >= maxAttempts {
			s.logger.Warn("Reset password locked out: max invalid OTP attempts reached", "email", email, "attempts", attempts)
			_ = s.resetStore.LockReset(ctx, email, lockoutDuration)
			return nil, appErrors.TooManyRequests("maximum invalid OTP attempts exceeded; password reset locked for 1 hour")
		}

		s.logger.Warn("Reset password failed: invalid OTP code", "email", email, "attempt", attempts)
		return nil, appErrors.BadRequest("invalid password reset code")
	}

	// 5. Look up user by email
	cred, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, appErrors.NotFound("user not found")
		}
		return nil, appErrors.Internal(err, "failed to query user credentials")
	}

	if !cred.IsActive {
		return nil, appErrors.Unauthorized("account is inactive")
	}

	// 6. Hash new password with bcrypt
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to hash new password")
	}

	// 7. Update password and invalidate sessions in database transaction
	evtPayload, _ := json.Marshal(map[string]any{
		"user_id":    cred.UserID.String(),
		"email":      cred.Email,
		"reset_at":   time.Now().UTC(),
		"reset_type": "otp",
	})
	outboxEvt := &outbox.Event{
		ID:            uuid.Must(uuid.NewV7()),
		AggregateType: "auth",
		AggregateID:   cred.UserID.String(),
		EventType:     "PasswordReset",
		Topic:         "auth.user.password_reset",
		Payload:       evtPayload,
	}

	if err := s.repo.UpdatePassword(ctx, cred.UserID, string(hashedPassword), outboxEvt); err != nil {
		return nil, appErrors.Internal(err, "failed to update password")
	}

	// 8. Consume (delete) the OTP immediately - single use
	_ = s.resetStore.DeleteResetOTP(ctx, email)

	// 9. Session & cache invalidation
	if s.redisClient != nil {
		_ = s.redisClient.Delete(ctx, failedLoginKey(email), accountLockKey(email))
	}

	s.logger.Info("Password reset successful", "user_id", cred.UserID.String(), "email", email)
	return &dto.ResetPasswordResponse{
		Success: true,
		Message: "Password has been reset successfully",
	}, nil
}

func (s *authService) ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) (*dto.ChangePasswordResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request cannot be empty")
	}

	userIDStr := strings.TrimSpace(req.UserID)
	if userIDStr == "" {
		if userCtx, ok := auth.FromContext(ctx); ok && userCtx != nil {
			userIDStr = userCtx.UserID
		}
	}
	if userIDStr == "" {
		return nil, appErrors.Unauthorized("authentication required")
	}

	targetUserID, err := uuid.FromString(userIDStr)
	if err != nil {
		return nil, appErrors.BadRequest("invalid user id format")
	}

	if req.OldPassword == "" {
		return nil, appErrors.BadRequest("current password is required")
	}
	if req.NewPassword == "" {
		return nil, appErrors.BadRequest("new password is required")
	}

	// Ensure newPassword is distinct from oldPassword
	if req.OldPassword == req.NewPassword {
		return nil, appErrors.BadRequest("new password must be distinct from current password")
	}

	// Validate password complexity
	if err := validator.ValidatePasswordStrength(req.NewPassword); err != nil {
		return nil, appErrors.BadRequest(err.Error())
	}

	// Query credential by user_id
	cred, err := s.repo.GetByUserID(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, appErrors.NotFound("user not found")
		}
		return nil, appErrors.Internal(err, "failed to query user credentials")
	}

	if !cred.IsActive {
		return nil, appErrors.Unauthorized("account is inactive")
	}

	// Verify oldPassword using bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(req.OldPassword)); err != nil {
		s.logger.Warn("Change password failed: incorrect old password", "user_id", cred.UserID.String())
		return nil, appErrors.InvalidCredentials("invalid current password")
	}

	// Hash new password using bcrypt
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to hash new password")
	}

	// Invalidate sessions and update password in DB
	evtPayload, _ := json.Marshal(map[string]any{
		"user_id":    cred.UserID.String(),
		"email":      cred.Email,
		"changed_at": time.Now().UTC(),
	})
	outboxEvt := &outbox.Event{
		ID:            uuid.Must(uuid.NewV7()),
		AggregateType: "auth",
		AggregateID:   cred.UserID.String(),
		EventType:     "PasswordChanged",
		Topic:         "auth.user.password_changed",
		Payload:       evtPayload,
	}

	if err := s.repo.UpdatePassword(ctx, cred.UserID, string(hashedPassword), outboxEvt); err != nil {
		return nil, appErrors.Internal(err, "failed to update password")
	}

	// Invalidate cache
	if s.redisClient != nil {
		_ = s.redisClient.Delete(ctx, failedLoginKey(cred.Email), accountLockKey(cred.Email))
	}

	s.logger.Info("Password changed successfully", "user_id", cred.UserID.String())
	return &dto.ChangePasswordResponse{
		Success: true,
		Message: "Password changed successfully",
	}, nil
}
