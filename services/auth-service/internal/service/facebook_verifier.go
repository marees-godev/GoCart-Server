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

type FacebookTokenPayload struct {
	UserID        string
	Email         string
	EmailVerified bool
	FirstName     string
	LastName      string
	Name          string
	AppID         string
	ExpiresAt     int64
}

type FacebookTokenVerifier interface {
	Verify(ctx context.Context, accessToken string) (*FacebookTokenPayload, error)
	ExchangeCode(ctx context.Context, code, redirectURI string) (string, error)
}

type HTTPFacebookTokenVerifier struct {
	appID      string
	appSecret  string
	httpClient *http.Client
}

func NewHTTPFacebookTokenVerifier(appID, appSecret string) *HTTPFacebookTokenVerifier {
	return &HTTPFacebookTokenVerifier{
		appID:     strings.TrimSpace(appID),
		appSecret: strings.TrimSpace(appSecret),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type fbDebugTokenResponse struct {
	Data struct {
		AppID     string `json:"app_id"`
		Type      string `json:"type"`
		IsValid   bool   `json:"is_valid"`
		ExpiresAt any    `json:"expires_at"`
		UserID    string `json:"user_id"`
		Error     *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

type fbMeResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Error     *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (v *HTTPFacebookTokenVerifier) ExchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	if v.appID == "" {
		return "", errors.New("facebook app ID is not configured")
	}
	if v.appSecret == "" {
		return "", errors.New("facebook app secret is not configured")
	}
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return "", errors.New("empty authorization code")
	}
	if unescaped, err := url.QueryUnescape(trimmedCode); err == nil && unescaped != "" {
		trimmedCode = unescaped
	}
	trimmedRedirect := strings.TrimSpace(redirectURI)
	if unescaped, err := url.QueryUnescape(trimmedRedirect); err == nil && unescaped != "" {
		trimmedRedirect = unescaped
	}

	endpoint := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/oauth/access_token?client_id=%s&client_secret=%s&redirect_uri=%s&code=%s",
		url.QueryEscape(v.appID),
		url.QueryEscape(v.appSecret),
		url.QueryEscape(trimmedRedirect),
		url.QueryEscape(trimmedCode),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create facebook token exchange request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute facebook token exchange: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode facebook token exchange response: %w", err)
	}
	if tokenResp.Error != nil {
		return "", fmt.Errorf("facebook token exchange failed: %s", tokenResp.Error.Message)
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("facebook token exchange did not return an access_token")
	}
	return tokenResp.AccessToken, nil
}

func (v *HTTPFacebookTokenVerifier) Verify(ctx context.Context, accessToken string) (*FacebookTokenPayload, error) {
	expectedAppID := strings.TrimSpace(v.appID)
	if expectedAppID == "" {
		return nil, errors.New("facebook app ID is not configured")
	}
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, errors.New("empty access token")
	}

	appToken := fmt.Sprintf("%s|%s", v.appID, v.appSecret)
	debugURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/debug_token?input_token=%s&access_token=%s",
		url.QueryEscape(token),
		url.QueryEscape(appToken),
	)

	debugReq, err := http.NewRequestWithContext(ctx, http.MethodGet, debugURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create facebook debug_token request: %w", err)
	}

	debugResp, err := v.httpClient.Do(debugReq)
	if err != nil {
		return nil, fmt.Errorf("execute facebook debug_token request: %w", err)
	}
	defer debugResp.Body.Close()

	var debugData fbDebugTokenResponse
	if err := json.NewDecoder(debugResp.Body).Decode(&debugData); err != nil {
		return nil, fmt.Errorf("decode facebook debug_token response: %w", err)
	}
	if debugData.Error != nil {
		return nil, fmt.Errorf("facebook debug_token error: %s", debugData.Error.Message)
	}
	if debugData.Data.Error != nil {
		return nil, fmt.Errorf("invalid facebook token: %s", debugData.Data.Error.Message)
	}
	if !debugData.Data.IsValid {
		return nil, errors.New("invalid facebook token")
	}
	if strings.TrimSpace(debugData.Data.AppID) != expectedAppID {
		return nil, fmt.Errorf("facebook token app ID mismatch: expected %s, got %s", expectedAppID, debugData.Data.AppID)
	}

	var expInt int64
	switch exp := debugData.Data.ExpiresAt.(type) {
	case float64:
		expInt = int64(exp)
	case int64:
		expInt = exp
	case string:
		expInt, _ = strconv.ParseInt(exp, 10, 64)
	}
	if expInt > 0 && time.Now().Unix() > expInt {
		return nil, errors.New("facebook token has expired")
	}
	if debugData.Data.UserID == "" {
		return nil, errors.New("facebook token missing user_id")
	}

	meURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/me?fields=id,name,first_name,last_name,email&access_token=%s",
		url.QueryEscape(token),
	)
	meReq, err := http.NewRequestWithContext(ctx, http.MethodGet, meURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create facebook profile request: %w", err)
	}
	meResp, err := v.httpClient.Do(meReq)
	if err != nil {
		return nil, fmt.Errorf("execute facebook profile request: %w", err)
	}
	defer meResp.Body.Close()

	var me fbMeResponse
	if err := json.NewDecoder(meResp.Body).Decode(&me); err != nil {
		return nil, fmt.Errorf("decode facebook profile response: %w", err)
	}
	if me.Error != nil {
		return nil, fmt.Errorf("facebook profile error: %s", me.Error.Message)
	}

	userID := debugData.Data.UserID
	if me.ID != "" {
		userID = me.ID
	}

	firstName := me.FirstName
	lastName := me.LastName
	if firstName == "" && lastName == "" && me.Name != "" {
		parts := strings.SplitN(strings.TrimSpace(me.Name), " ", 2)
		firstName = parts[0]
		if len(parts) > 1 {
			lastName = parts[1]
		}
	}

	email := strings.TrimSpace(me.Email)
	emailVerified := email != ""

	return &FacebookTokenPayload{
		UserID:        userID,
		Email:         email,
		EmailVerified: emailVerified,
		FirstName:     firstName,
		LastName:      lastName,
		Name:          me.Name,
		AppID:         debugData.Data.AppID,
		ExpiresAt:     expInt,
	}, nil
}
