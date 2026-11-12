package resolvers

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

	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) getAuthClient() (authpb.AuthServiceClient, error) {
	if r == nil {
		return nil, appErrors.Internal(nil, "auth client unavailable")
	}
	if r.Clients != nil && r.Clients.AuthClient != nil {
		return r.Clients.AuthClient, nil
	}
	if r.ClientMgr != nil && r.ClientMgr.AuthClient != nil {
		return r.ClientMgr.AuthClient, nil
	}
	return nil, appErrors.Internal(nil, "auth client unavailable")
}

func (r *mutationResolver) getAuthClient() (authpb.AuthServiceClient, error) {
	if r == nil || r.Resolver == nil {
		return nil, appErrors.Internal(nil, "auth client unavailable")
	}
	return r.Resolver.getAuthClient()
}

func (r *Resolver) getDeliveryClient() (deliverypb.DeliveryServiceClient, error) {
	if r == nil {
		return nil, appErrors.ServiceUnavailable("delivery service unavailable")
	}
	if r.Clients != nil && r.Clients.DeliveryClient != nil {
		return r.Clients.DeliveryClient, nil
	}
	if r.ClientMgr != nil && r.ClientMgr.DeliveryClient != nil {
		return r.ClientMgr.DeliveryClient, nil
	}
	return nil, appErrors.ServiceUnavailable("delivery service unavailable")
}

func (r *mutationResolver) getDeliveryClient() (deliverypb.DeliveryServiceClient, error) {
	if r == nil || r.Resolver == nil {
		return nil, appErrors.ServiceUnavailable("delivery service unavailable")
	}
	return r.Resolver.getDeliveryClient()
}

var (
	googleTokenEndpoint = "https://oauth2.googleapis.com/token"
	googleHTTPClient    = &http.Client{Timeout: 10 * time.Second}
)

func exchangeGoogleAuthCode(ctx context.Context, clientID, clientSecret, redirectURI, code string) (string, error) {
	if clientID == "" {
		return "", errors.New("GOOGLE_CLIENT_ID is not configured on the server")
	}
	if clientSecret == "" {
		return "", errors.New("GOOGLE_CLIENT_SECRET is required to exchange authorization code")
	}
	if unescaped, err := url.QueryUnescape(code); err == nil && unescaped != "" {
		code = unescaped
	}
	if unescaped, err := url.QueryUnescape(redirectURI); err == nil && unescaped != "" {
		redirectURI = unescaped
	}
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("redirect_uri", redirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := googleHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute token exchange: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var tokenResp struct {
		IDToken   string `json:"id_token"`
		Error     string `json:"error"`
		ErrorDesc string `json:"error_description"`
	}
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return "", errors.New("failed to parse Google token exchange response")
	}
	if tokenResp.Error != "" {
		if tokenResp.ErrorDesc != "" {
			return "", fmt.Errorf("google token exchange failed: %s (%s)", tokenResp.Error, tokenResp.ErrorDesc)
		}
		return "", fmt.Errorf("google token exchange failed: %s", tokenResp.Error)
	}
	if tokenResp.IDToken == "" {
		return "", errors.New("google token exchange did not return an id_token")
	}
	return tokenResp.IDToken, nil
}
