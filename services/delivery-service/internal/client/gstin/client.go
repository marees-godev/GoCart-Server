package gstin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/config"
)

var ErrGSTINVerificationDisabled = errors.New("GSTIN verification is disabled")
var GSTINRegex = regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z]{1}[1-9A-Z]{1}Z[0-9A-Z]{1}$`)
var gstinRegex = GSTINRegex

type AddressDetails struct {
	BuildingNumber string `json:"building_number,omitempty"`
	BuildingName   string `json:"building_name,omitempty"`
	Floor          string `json:"floor,omitempty"`
	Street         string `json:"street,omitempty"`
	Locality       string `json:"locality,omitempty"`
	District       string `json:"district,omitempty"`
	City           string `json:"city,omitempty"`
	State          string `json:"state,omitempty"`
	Landmark       string `json:"landmark,omitempty"`
	Pincode        string `json:"pincode,omitempty"`
}

type GSTINData struct {
	GSTIN                string          `json:"gstin"`
	LegalName            string          `json:"legal_name"`
	TradeName            string          `json:"trade_name"`
	Status               string          `json:"status"`
	TaxpayerType         string          `json:"taxpayer_type"`
	BusinessConstitution string          `json:"business_constitution,omitempty"`
	RegistrationDate     string          `json:"registration_date"`
	CancellationDate     string          `json:"cancellation_date,omitempty"`
	StateCode            string          `json:"state_code"`
	StateJurisdiction    string          `json:"state_jurisdiction,omitempty"`
	Address              string          `json:"address"`
	City                 string          `json:"city"`
	AddressDetails       *AddressDetails `json:"address_details,omitempty"`
	Pincode              string          `json:"pincode"`
	NatureOfBusiness     string          `json:"nature_of_business,omitempty"`
	BlockStatus          string          `json:"block_status"`
}

type GSTINResponse struct {
	Success          bool       `json:"success"`
	GSTIN            string     `json:"gstin"`
	Data             *GSTINData `json:"data,omitempty"`
	Message          string     `json:"message,omitempty"`
	Error            string     `json:"error,omitempty"`
	BilledTo         string     `json:"billed_to,omitempty"`
	CreditsRemaining int        `json:"credits_remaining,omitempty"`
	ResponseMS       int        `json:"response_ms,omitempty"`
}

type GSTINClient interface {
	VerifyGSTIN(ctx context.Context, gstin string) (*GSTINResponse, error)
	IsEnabled() bool
}

type client struct {
	apiKey     string
	baseURL    string
	enabled    bool
	httpClient *http.Client
}

func NewGSTINClient(cfg config.GSTINConfig, customHTTP ...*http.Client) GSTINClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://www.gstinapi.in/v1"
	}
	httpClient := &http.Client{
		Timeout: timeout,
	}
	if len(customHTTP) > 0 && customHTTP[0] != nil {
		httpClient = customHTTP[0]
	}
	return &client{
		apiKey:     cfg.APIKey,
		baseURL:    baseURL,
		enabled:    cfg.Enabled,
		httpClient: httpClient,
	}
}

func (c *client) IsEnabled() bool {
	return c.enabled
}

func (c *client) VerifyGSTIN(ctx context.Context, gstin string) (*GSTINResponse, error) {
	if !c.enabled {
		return nil, ErrGSTINVerificationDisabled
	}
	gstin = strings.ToUpper(strings.TrimSpace(gstin))
	if !gstinRegex.MatchString(gstin) {
		return nil, fmt.Errorf("invalid GSTIN format")
	}

	if strings.TrimSpace(c.apiKey) == "" {
		return nil, fmt.Errorf("invalid credential: GSTIN API authentication failed (GSTIN_API_KEY is not configured)")
	}

	reqURL := fmt.Sprintf("%s/gstin/%s", c.baseURL, url.PathEscape(gstin))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GSTIN request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GSTIN API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("invalid credential: GSTIN API authentication failed")
	}

	if resp.StatusCode != http.StatusOK {
		var errRes GSTINResponse
		if err := json.NewDecoder(resp.Body).Decode(&errRes); err == nil {
			msg := errRes.Message
			if msg == "" {
				msg = errRes.Error
			}
			if msg != "" {
				return nil, fmt.Errorf("GSTIN verification failed: %s", msg)
			}
		}
		return nil, fmt.Errorf("GSTIN API returned HTTP status %d", resp.StatusCode)
	}

	var res GSTINResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode GSTIN API response: %w", err)
	}

	return &res, nil
}

