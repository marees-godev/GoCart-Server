package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
)

type AuthHandler struct {
	cfg        *config.Config
	authClient authpb.AuthServiceClient
	httpClient *http.Client
	logger     *slog.Logger
}

func NewAuthHandler(cfg *config.Config, authClient authpb.AuthServiceClient, log *slog.Logger) *AuthHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AuthHandler{
		cfg:        cfg,
		authClient: authClient,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     log,
	}
}

func (h *AuthHandler) RegisterRoutes(app fiber.Router) {
	group := app.Group("/api/v1/auth/google")
	group.Get("", h.HandleGoogleLogin)
	group.Get("/login", h.HandleGoogleLogin)
	group.Get("/register", h.HandleGoogleRegisterRedirect)
	group.Post("/login", h.HandleGoogleLoginPost)
	group.Post("/register", h.HandleGoogleRegisterPost)
	group.Get("/callback", h.HandleGoogleCallback)
	group.Post("/callback", h.HandleGoogleCallback)
}

func (h *AuthHandler) buildGoogleAuthURL(c *fiber.Ctx, defaultAction string) (string, string, error) {
	clientID := strings.TrimSpace(h.cfg.Google.ClientID)
	if clientID == "" {
		return "", "", errors.New("GOOGLE_CLIENT_ID is not configured on the server")
	}

	redirectURI := strings.TrimSpace(c.Query("redirect_uri"))
	if redirectURI == "" {
		redirectURI = strings.TrimSpace(h.cfg.Google.RedirectURL)
	}
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s://%s/api/v1/auth/google/callback", c.Protocol(), c.Hostname())
	}

	role := c.Query("role", "USER")
	action := c.Query("action", defaultAction)
	state := role
	if action != "" {
		state = fmt.Sprintf("%s:%s", role, strings.ToLower(action))
	}

	scope := strings.TrimSpace(h.cfg.Google.Scopes)
	if scope == "" {
		scope = "openid email profile"
	}
	googleAuthURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent&state=%s",
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(scope),
		url.QueryEscape(state),
	)

	return googleAuthURL, redirectURI, nil
}

func (h *AuthHandler) HandleGoogleLogin(c *fiber.Ctx) error {
	googleAuthURL, _, err := h.buildGoogleAuthURL(c, "login")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "config_error",
			"message": err.Error(),
		})
	}

	return c.Redirect(googleAuthURL, fiber.StatusTemporaryRedirect)
}

func (h *AuthHandler) HandleGoogleRegisterRedirect(c *fiber.Ctx) error {
	googleAuthURL, _, err := h.buildGoogleAuthURL(c, "register")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "config_error",
			"message": err.Error(),
		})
	}

	return c.Redirect(googleAuthURL, fiber.StatusTemporaryRedirect)
}

func (h *AuthHandler) HandleGoogleLoginPost(c *fiber.Ctx) error {
	return h.handleGoogleAuthFlow(c, "login")
}

func (h *AuthHandler) HandleGoogleRegisterPost(c *fiber.Ctx) error {
	return h.handleGoogleAuthFlow(c, "register")
}

type googleTokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (h *AuthHandler) HandleGoogleCallback(c *fiber.Ctx) error {
	if errParam := c.Query("error"); errParam != "" {
		desc := c.Query("error_description", "Access denied by user or Google")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             errParam,
			"error_description": desc,
		})
	}

	action := "login"
	state := strings.ToLower(strings.TrimSpace(c.Query("state")))
	if strings.Contains(state, ":register") || state == "register" {
		action = "register"
	}

	return h.handleGoogleAuthFlow(c, action)
}

func (h *AuthHandler) handleGoogleAuthFlow(c *fiber.Ctx, action string) error {
	var reqBody struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
		Role        string `json:"role"`
	}
	if c.Method() == fiber.MethodPost {
		_ = c.BodyParser(&reqBody)
	}

	code := strings.TrimSpace(c.Query("code"))
	if code == "" && reqBody.Code != "" {
		code = strings.TrimSpace(reqBody.Code)
	}

	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "invalid_request",
			"message": "code is required",
		})
	}

	clientID := strings.TrimSpace(h.cfg.Google.ClientID)
	clientSecret := strings.TrimSpace(h.cfg.Google.ClientSecret)
	redirectURI := strings.TrimSpace(c.Query("redirect_uri"))
	if redirectURI == "" && reqBody.RedirectURI != "" {
		redirectURI = strings.TrimSpace(reqBody.RedirectURI)
	}
	if redirectURI == "" {
		redirectURI = strings.TrimSpace(h.cfg.Google.RedirectURL)
	}
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s://%s/api/v1/auth/google/callback", c.Protocol(), c.Hostname())
	}

	if clientID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "config_error",
			"message": "GOOGLE_CLIENT_ID is not configured on the server",
		})
	}
	if clientSecret == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "config_error",
			"message": "GOOGLE_CLIENT_SECRET is required to exchange authorization code for tokens",
		})
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

	req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "request_error",
			"message": fmt.Sprintf("failed to create token exchange request: %v", err),
		})
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "google_network_error",
			"message": fmt.Sprintf("failed to reach Google token endpoint: %v", err),
		})
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var tokenResp googleTokenExchangeResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "decode_error",
			"message": "failed to parse Google token exchange response",
		})
	}

	if tokenResp.Error != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             tokenResp.Error,
			"error_description": tokenResp.ErrorDesc,
		})
	}

	if tokenResp.IDToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "invalid_response",
			"message": "Google did not return an id_token",
		})
	}

	rawState := strings.TrimSpace(c.Query("state"))
	roleStr := strings.ToUpper(rawState)
	if strings.Contains(roleStr, ":") {
		roleStr = strings.Split(roleStr, ":")[0]
	}
	if roleStr == "" && reqBody.Role != "" {
		roleStr = strings.ToUpper(strings.TrimSpace(reqBody.Role))
	}
	role := authpb.Role_USER
	switch roleStr {
	case "MERCHANT":
		role = authpb.Role_MERCHANT
	case "DELIVERY_PARTNER", "DELIVERY":
		role = authpb.Role_DELIVERY_PARTNER
	}

	if h.authClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "service_unavailable",
			"message": "Auth microservice connection is not available",
		})
	}

	authReq := &authpb.GoogleAuthRequest{
		IdToken: tokenResp.IDToken,
		Role:    role,
	}

	var authResp *authpb.AuthResponse
	if action == "register" {
		authResp, err = h.authClient.GoogleRegister(c.Context(), authReq)
	} else {
		authResp, err = h.authClient.GoogleLogin(c.Context(), authReq)
	}

	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "authentication_failed",
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("Google %s successful", action),
		"tokens": fiber.Map{
			"access_token":  authResp.AccessToken,
			"refresh_token": authResp.RefreshToken,
			"token_type":    authResp.TokenType,
			"expires_in":    authResp.ExpiresIn,
			"user_id":       authResp.UserId,
			"role":          authResp.Role.String(),
			"email":         authResp.BusinessEmail,
			"first_name":    authResp.FirstName,
			"last_name":     authResp.LastName,
			"merchant_id":   authResp.MerchantId,
			"partner_id":    authResp.PartnerId,
			"customer_id":   authResp.CustomerId,
		},
	})
}
