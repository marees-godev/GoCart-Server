package provider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestIndiaPostProvider_Success(t *testing.T) {
	mockResponse := `[
		{
			"Message": "Number of pincode found:2",
			"Status": "Success",
			"PostOffice": [
				{
					"Name": "Mandapam Camp",
					"BranchType": "Sub Post Office",
					"DeliveryStatus": "Delivery",
					"Circle": "Tamilnadu",
					"District": "Ramanathapuram",
					"Division": "Ramanathapuram",
					"Region": "Madurai",
					"State": "Tamil Nadu",
					"Country": "India",
					"Pincode": "623517"
				},
				{
					"Name": "Mandapam",
					"BranchType": "Sub Post Office",
					"DeliveryStatus": "Non-Delivery",
					"Circle": "Tamilnadu",
					"District": "Ramanathapuram",
					"Division": "Ramanathapuram",
					"Region": "Madurai",
					"State": "Tamil Nadu",
					"Country": "India",
					"Pincode": "623517"
				}
			]
		}
	]`

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockResponse)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	prov := NewIndiaPostProviderWithClient("https://api.postalpincode.in/pincode/", client)
	loc, err := prov.LookupPincode(context.Background(), "623517")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if loc.Pincode != "623517" {
		t.Errorf("expected pincode 623517, got %s", loc.Pincode)
	}
	if loc.Country != "India" {
		t.Errorf("expected country India, got %s", loc.Country)
	}
	if loc.State != "Tamil Nadu" {
		t.Errorf("expected state Tamil Nadu, got %s", loc.State)
	}
	if loc.District != "Ramanathapuram" {
		t.Errorf("expected district Ramanathapuram, got %s", loc.District)
	}
	if len(loc.PostOffices) != 2 {
		t.Fatalf("expected 2 post offices, got %d", len(loc.PostOffices))
	}
	if loc.PostOffices[0].Name != "Mandapam Camp" || loc.PostOffices[1].Name != "Mandapam" {
		t.Errorf("unexpected post office names: %v, %v", loc.PostOffices[0].Name, loc.PostOffices[1].Name)
	}
}

func TestIndiaPostProvider_NotFound(t *testing.T) {
	mockResponse := `[
		{
			"Message": "No records found",
			"Status": "Error",
			"PostOffice": null
		}
	]`

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockResponse)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	prov := NewIndiaPostProviderWithClient("https://api.postalpincode.in/pincode/", client)
	_, err := prov.LookupPincode(context.Background(), "999999")
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("expected ErrLocationNotFound, got %v", err)
	}
}

func TestIndiaPostProvider_HTTP404(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	prov := NewIndiaPostProviderWithClient("https://api.postalpincode.in/pincode/", client)
	_, err := prov.LookupPincode(context.Background(), "999999")
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("expected ErrLocationNotFound, got %v", err)
	}
}

func TestIndiaPostProvider_ServerError(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString("server error")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	prov := NewIndiaPostProviderWithClient("https://api.postalpincode.in/pincode/", client)
	_, err := prov.LookupPincode(context.Background(), "623517")
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}

func TestIndiaPostProvider_Timeout(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	}

	prov := NewIndiaPostProviderWithClient("https://api.postalpincode.in/pincode/", client)
	_, err := prov.LookupPincode(context.Background(), "623517")
	if !errors.Is(err, ErrProviderTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestIndiaPostProvider_InvalidJSON(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("invalid json")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	prov := NewIndiaPostProviderWithClient("https://api.postalpincode.in/pincode/", client)
	_, err := prov.LookupPincode(context.Background(), "623517")
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}
