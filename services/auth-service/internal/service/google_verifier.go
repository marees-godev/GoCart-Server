package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type GoogleTokenPayload struct {
	Subject       string
	Email         string
	EmailVerified bool
	FirstName     string
	LastName      string
	Name          string
	Picture       string
	Audience      string
	Issuer        string
	ExpiresAt     int64
}

type GoogleTokenVerifier interface {
	Verify(ctx context.Context, idToken string) (*GoogleTokenPayload, error)
}

type HTTPGoogleTokenVerifier struct {
	clientID   string
	httpClient *http.Client
}

func NewHTTPGoogleTokenVerifier(clientID string) *HTTPGoogleTokenVerifier {
	return &HTTPGoogleTokenVerifier{
		clientID: clientID,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type tokenInfoResponse struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
	FirstName     string `json:"given_name"`
	LastName      string `json:"family_name"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Audience      string `json:"aud"`
	Issuer        string `json:"iss"`
	ExpiresAt     any    `json:"exp"`
	Error         string `json:"error"`
	ErrorDesc     string `json:"error_description"`
}

func (v *HTTPGoogleTokenVerifier) Verify(ctx context.Context, idToken string) (*GoogleTokenPayload, error) {
	expectedClientID := strings.TrimSpace(v.clientID)
	if expectedClientID == "" {
		return nil, errors.New("google client ID is not configured")
	}

	token := strings.TrimSpace(idToken)
	if token == "" {
		return nil, errors.New("empty id token")
	}

	endpoint := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create google tokeninfo request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute google tokeninfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp tokenInfoResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.ErrorDesc != "" {
			return nil, fmt.Errorf("invalid google token: %s", errResp.ErrorDesc)
		}
		if errResp.Error != "" {
			return nil, fmt.Errorf("invalid google token: %s", errResp.Error)
		}
		return nil, fmt.Errorf("google token verification failed with status: %d", resp.StatusCode)
	}

	var raw tokenInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode google token response: %w", err)
	}

	if raw.Subject == "" {
		return nil, errors.New("google token missing sub claim")
	}
	if raw.Email == "" {
		return nil, errors.New("google token missing email claim")
	}

	emailVerified := false
	switch ev := raw.EmailVerified.(type) {
	case bool:
		emailVerified = ev
	case string:
		emailVerified = strings.EqualFold(ev, "true")
	}

	if !emailVerified {
		return nil, errors.New("google email is not verified")
	}

	var expInt int64
	switch exp := raw.ExpiresAt.(type) {
	case float64:
		expInt = int64(exp)
	case string:
		expInt, _ = strconv.ParseInt(exp, 10, 64)
	}

	if expInt > 0 && time.Now().Unix() > expInt {
		return nil, errors.New("google token has expired")
	}

	if strings.TrimSpace(raw.Audience) == "" || raw.Audience != expectedClientID {
		return nil, fmt.Errorf("google token client ID mismatch: expected %s, got %s", expectedClientID, raw.Audience)
	}

	if raw.Issuer != "https://accounts.google.com" && raw.Issuer != "accounts.google.com" {
		return nil, fmt.Errorf("google token invalid issuer: %s", raw.Issuer)
	}

	firstName := raw.FirstName
	lastName := raw.LastName
	if firstName == "" && lastName == "" && raw.Name != "" {
		parts := strings.SplitN(strings.TrimSpace(raw.Name), " ", 2)
		firstName = parts[0]
		if len(parts) > 1 {
			lastName = parts[1]
		}
	}

	return &GoogleTokenPayload{
		Subject:       raw.Subject,
		Email:         raw.Email,
		EmailVerified: emailVerified,
		FirstName:     firstName,
		LastName:      lastName,
		Name:          raw.Name,
		Picture:       raw.Picture,
		Audience:      raw.Audience,
		Issuer:        raw.Issuer,
		ExpiresAt:     expInt,
	}, nil
}

