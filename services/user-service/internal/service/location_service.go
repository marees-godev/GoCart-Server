package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/validation"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/provider"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/repository"
)

type LocationService interface {
	GetLocationByPincode(ctx context.Context, pincode string, countryCode string) (*model.Location, error)
}

type locationService struct {
	repo     repository.LocationRepository
	provider provider.LocationProvider
}

func NewLocationService(repo repository.LocationRepository, prov provider.LocationProvider) LocationService {
	return &locationService{
		repo:     repo,
		provider: prov,
	}
}

func (s *locationService) GetLocationByPincode(ctx context.Context, pincode string, countryCode string) (*model.Location, error) {
	cleanPincode := strings.TrimSpace(pincode)
	if cleanPincode == "" {
		return nil, appErrors.BadRequest("pincode is required")
	}

	cleanCountry := strings.ToUpper(strings.TrimSpace(countryCode))
	if cleanCountry == "" {
		cleanCountry = "IN"
	}

	if cleanCountry == "IN" || cleanCountry == "IND" || cleanCountry == "INDIA" {
		if err := validation.ValidateIndianPincode(cleanPincode); err != nil {
			return nil, err
		}
	} else {
		if err := validation.ValidatePostalCode(cleanPincode, cleanCountry); err != nil {
			return nil, err
		}
	}

	cachedLoc, err := s.repo.GetByPincode(ctx, cleanPincode)
	if err != nil {
		slog.WarnContext(ctx, "failed to query location cache from database", "pincode", cleanPincode, "error", err)
	} else if cachedLoc != nil {
		return cachedLoc, nil
	}

	loc, err := s.provider.LookupPincode(ctx, cleanPincode)
	if err != nil {
		if errors.Is(err, provider.ErrLocationNotFound) {
			return nil, appErrors.NotFound(fmt.Sprintf("no location details found for PIN code %s", cleanPincode))
		}
		if errors.Is(err, provider.ErrProviderTimeout) {
			slog.ErrorContext(ctx, "pincode provider timeout", "pincode", cleanPincode, "error", err)
			return nil, appErrors.ServiceUnavailable("location lookup provider timed out, please try again")
		}
		if errors.Is(err, provider.ErrProviderUnavailable) {
			slog.ErrorContext(ctx, "pincode provider unavailable", "pincode", cleanPincode, "error", err)
			return nil, appErrors.ServiceUnavailable("location lookup provider is currently unavailable")
		}
		slog.ErrorContext(ctx, "external pincode lookup failed", "pincode", cleanPincode, "error", err)
		return nil, appErrors.Internal(err, "failed to lookup location details")
	}

	if saveErr := s.repo.Save(ctx, loc); saveErr != nil {
		slog.WarnContext(ctx, "failed to cache location in database", "pincode", cleanPincode, "error", saveErr)
	}

	return loc, nil
}
