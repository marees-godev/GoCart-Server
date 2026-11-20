package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
)

var (
	ErrLocationNotFound    = errors.New("location not found for given pincode")
	ErrProviderUnavailable = errors.New("external pincode provider is unavailable")
	ErrProviderTimeout     = errors.New("external pincode provider request timed out")
	ErrInvalidResponse     = errors.New("invalid response from pincode provider")
)

type LocationProvider interface {
	LookupPincode(ctx context.Context, pincode string) (*model.Location, error)
}

type IndiaPostOffice struct {
	Name           string `json:"Name"`
	Description    *string `json:"Description"`
	BranchType     string `json:"BranchType"`
	DeliveryStatus string `json:"DeliveryStatus"`
	Circle         string `json:"Circle"`
	District       string `json:"District"`
	Division       string `json:"Division"`
	Region         string `json:"Region"`
	Block          string `json:"Block"`
	State          string `json:"State"`
	Country        string `json:"Country"`
	Pincode        string `json:"Pincode"`
}

type IndiaPostResponseItem struct {
	Message    string            `json:"Message"`
	Status     string            `json:"Status"`
	PostOffice []IndiaPostOffice `json:"PostOffice"`
}

type IndiaPostProvider struct {
	baseURL string
	client  *http.Client
}

func NewIndiaPostProvider(baseURL string, timeout time.Duration) *IndiaPostProvider {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return NewIndiaPostProviderWithClient(baseURL, &http.Client{Timeout: timeout})
}

func NewIndiaPostProviderWithClient(baseURL string, client *http.Client) *IndiaPostProvider {
	if baseURL == "" {
		baseURL = "https://api.postalpincode.in/pincode/"
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	return &IndiaPostProvider{
		baseURL: baseURL,
		client:  client,
	}
}

func (p *IndiaPostProvider) LookupPincode(ctx context.Context, pincode string) (*model.Location, error) {
	reqURL := p.baseURL + url.PathEscape(pincode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GoCart-LocationService/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ErrProviderTimeout
		}
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrLocationNotFound
	}
	if resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
		return nil, ErrProviderUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected HTTP status %d", ErrProviderUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}

	var items []IndiaPostResponseItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	if len(items) == 0 {
		return nil, ErrLocationNotFound
	}

	item := items[0]
	if !strings.EqualFold(item.Status, "Success") || len(item.PostOffice) == 0 {
		return nil, ErrLocationNotFound
	}

	firstPO := item.PostOffice[0]
	country := firstPO.Country
	state := firstPO.State
	district := firstPO.District

	postOffices := make([]model.PostOffice, 0, len(item.PostOffice))
	for _, po := range item.PostOffice {
		postOffices = append(postOffices, model.PostOffice{
			Name:           po.Name,
			BranchType:     po.BranchType,
			DeliveryStatus: po.DeliveryStatus,
			Circle:         po.Circle,
			District:       po.District,
			Division:       po.Division,
			Region:         po.Region,
			State:          po.State,
			Country:        po.Country,
			Pincode:        po.Pincode,
		})
	}

	return &model.Location{
		Pincode:     pincode,
		Country:     country,
		State:       state,
		District:    district,
		PostOffices: postOffices,
	}, nil
}
