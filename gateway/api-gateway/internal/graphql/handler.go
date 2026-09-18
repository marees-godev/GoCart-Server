package graphql

import (
	"context"
	"net/http"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	gqlHandler "github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	pkgGraphQL "github.com/marees-godev/GoCart-Server/pkg/graphql"
)

type contextKey string

const (
	userIDKey   contextKey = "userID"
	userRoleKey contextKey = "userRole"
)

type GraphQLRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

type Handler struct {
	server http.Handler
	cfg    *config.Config
}

func NewHandler(es graphql.ExecutableSchema, cfg *config.Config) *Handler {
	srv := gqlHandler.NewDefaultServer(es)
	srv.SetErrorPresenter(func(ctx context.Context, e error) *gqlerror.Error {
		err := graphql.DefaultErrorPresenter(ctx, e)
		appErr := appErrors.AsAppError(e)
		if appErr != nil {
			if err.Extensions == nil {
				err.Extensions = make(map[string]interface{})
			}
			err.Extensions["code"] = appErr.Code
			err.Message = appErr.ClientMessage()
		}
		return err
	})

	return &Handler{
		server: srv,
		cfg:    cfg,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authHeader := r.Header.Get("Authorization")
	secret := "gocart-secret-key-change-in-production"
	if h.cfg != nil && h.cfg.JWT.Secret != "" {
		secret = h.cfg.JWT.Secret
	}

	if authHeader != "" {
		if userCtx, err := auth.ValidateToken(authHeader, secret); err == nil && userCtx != nil {
			ctx = auth.WithUser(ctx, userCtx)
			ctx = context.WithValue(ctx, userIDKey, userCtx.UserID)
			ctx = context.WithValue(ctx, userRoleKey, userCtx.Role)
			r = r.WithContext(ctx)
		}
	}

	h.server.ServeHTTP(w, r)
}

func (h *Handler) HandleQuery(c *fiber.Ctx) error {
	ctx := c.UserContext()
	if ctx == nil {
		ctx = c.Context()
	}

	authHeader := c.Get("Authorization")
	secret := "gocart-secret-key-change-in-production"
	if h.cfg != nil && h.cfg.JWT.Secret != "" {
		secret = h.cfg.JWT.Secret
	}

	if authHeader != "" {
		if userCtx, err := auth.ValidateToken(authHeader, secret); err == nil && userCtx != nil {
			ctx = auth.WithUser(ctx, userCtx)
			ctx = context.WithValue(ctx, userIDKey, userCtx.UserID)
			ctx = context.WithValue(ctx, userRoleKey, userCtx.Role)
			c.SetUserContext(ctx)
		}
	} else if userCtx, ok := auth.FromContext(ctx); ok && userCtx != nil {
		ctx = context.WithValue(ctx, userIDKey, userCtx.UserID)
		ctx = context.WithValue(ctx, userRoleKey, userCtx.Role)
		c.SetUserContext(ctx)
	} else {
		if legacyID, ok := ctx.Value(userIDKey).(string); ok && legacyID != "" {
			legacyRole, _ := ctx.Value(userRoleKey).(string)
			userCtx := &auth.UserContext{
				UserID: legacyID,
				Role:   legacyRole,
			}
			ctx = auth.WithUser(ctx, userCtx)
			c.SetUserContext(ctx)
		}
	}

	if h.cfg != nil && !h.cfg.GraphQLIntrospectionEnabled {
		body := string(c.Body())
		if strings.Contains(body, "__schema") || strings.Contains(body, "__type") {
			appErr := appErrors.Forbidden("GraphQL introspection is disabled")
			gqlErr := pkgGraphQL.FormatError(ctx, appErr)
			return c.Status(http.StatusOK).JSON(pkgGraphQL.GraphQLErrorResponse{
				Errors: []pkgGraphQL.GraphQLError{gqlErr},
			})
		}
	}

	return adaptor.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(ctx)
		h.server.ServeHTTP(w, r)
	}))(c)
}

func (h *Handler) HandlePlayground(c *fiber.Ctx) error {
	if h.cfg != nil && !h.cfg.GraphQLIntrospectionEnabled {
		return c.Status(http.StatusForbidden).SendString("GraphQL Introspection/Playground is disabled")
	}
	return adaptor.HTTPHandler(playground.Handler("GoCart API Gateway GraphQL", "/query"))(c)
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Post("/query", h.HandleQuery)
	app.Get("/query", h.HandlePlayground)
	app.Get("/playground", h.HandlePlayground)
	app.Post("/graphql", h.HandleQuery)
	app.Get("/graphql", h.HandlePlayground)
}
