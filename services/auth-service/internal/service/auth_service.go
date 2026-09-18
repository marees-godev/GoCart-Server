package service

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type AuthService interface {
	Register(ctx context.Context, req dto.RegisterRequest) (*dto.RegisterResponse, error)
}

type authService struct {
	repo repository.AuthRepository
}

func NewAuthService(repo repository.AuthRepository) AuthService {
	return &authService{
		repo: repo,
	}
}

func (s *authService) Register(ctx context.Context, req dto.RegisterRequest) (*dto.RegisterResponse, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if normalizedEmail == "" {
		return nil, errors.BadRequest("Email is required")
	}
	if _, err := mail.ParseAddress(normalizedEmail); err != nil {
		return nil, errors.BadRequest("Invalid email address format")
	}

	if len(req.Password) < 8 {
		return nil, errors.BadRequest("Password must be at least 8 characters long")
	}

	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if role == "ADMIN" {
		return nil, errors.BadRequest("Public registration for ADMIN is not allowed")
	}
	if role != "CUSTOMER" && role != "MERCHANT" {
		return nil, errors.BadRequest("Invalid role specified")
	}

	phone := strings.TrimSpace(req.Phone)

	existingEmail, err := s.repo.GetByEmail(ctx, normalizedEmail)
	if err != nil {
		return nil, errors.Internal(err, "Failed to check email availability")
	}
	if existingEmail != nil {
		return nil, errors.Conflict("Email is already registered")
	}

	if phone != "" {
		existingPhone, err := s.repo.GetByPhone(ctx, phone)
		if err != nil {
			return nil, errors.Internal(err, "Failed to check phone availability")
		}
		if existingPhone != nil {
			return nil, errors.Conflict("Phone number is already registered")
		}
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Internal(err, "Failed to hash password")
	}

	id := uuid.New()
	userID := uuid.New()
	now := time.Now().UTC()

	var phonePtr *string
	if phone != "" {
		phonePtr = &phone
	}

	cred := &model.AuthCredential{
		ID:               id,
		UserID:           userID,
		Email:            normalizedEmail,
		Phone:            phonePtr,
		PasswordHash:     string(hashedPassword),
		Role:             role,
		EmailVerified:    false,
		IsActive:         true,
		FailedLoginCount: 0,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	evtPayload := events.UserRegisteredEvent{
		UserID:    userID.String(),
		Email:     normalizedEmail,
		Phone:     phone,
		Role:      role,
		CreatedAt: now,
	}

	envelope, err := events.NewEventEnvelope(events.EventTypeUserRegistered, "auth-service", evtPayload)
	if err != nil {
		return nil, errors.Internal(err, "Failed to create event envelope")
	}

	envelopeBytes, err := envelope.Marshal()
	if err != nil {
		return nil, errors.Internal(err, "Failed to marshal event envelope")
	}

	outboxEvt := &outbox.Event{
		AggregateType: "user",
		AggregateID:   userID.String(),
		EventType:     events.EventTypeUserRegistered,
		Payload:       envelopeBytes,
		Topic:         "user-events",
	}

	if err := s.repo.CreateWithOutbox(ctx, cred, outboxEvt); err != nil {
		return nil, errors.Internal(err, "Failed to create user credential record")
	}

	return &dto.RegisterResponse{
		ID:            id,
		UserID:        userID,
		Email:         normalizedEmail,
		Phone:         phonePtr,
		Role:          role,
		EmailVerified: false,
		CreatedAt:     now,
	}, nil
}
