package graphql

import (
	"context"
	"net/http"
	"strings"
	"time"

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
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
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

	srv.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		opName := "anonymous"
		if oc != nil {
			if oc.OperationName != "" {
				opName = oc.OperationName
			} else if oc.Operation != nil && oc.Operation.Name != "" {
				opName = oc.Operation.Name
			}
		}

		start := time.Now()
		resHandler := next(ctx)
		duration := time.Since(start)

		var userID, role string
		if userCtx, ok := auth.UserFromContext(ctx); ok && userCtx != nil {
			userID = userCtx.UserID
			role = userCtx.Role
		} else {
			if uID, ok := ctx.Value(userIDKey).(string); ok {
				userID = uID
			}
			if r, ok := ctx.Value(userRoleKey).(string); ok {
				role = r
			}
		}

		reqID := middleware.GetRequestID(ctx)

		logger.FromContext(ctx).Info("GraphQL operation completed",
			"operation_name", opName,
			"request_id", reqID,
			"user_id", userID,
			"role", role,
			"duration", duration.String(),
			"duration_ms", float64(duration.Microseconds())/1000.0,
		)

		return resHandler
	})

	srv.SetErrorPresenter(func(ctx context.Context, e error) *gqlerror.Error {
		err := graphql.DefaultErrorPresenter(ctx, e)
		appErr := appErrors.AsAppError(e)
		errorCode := "INTERNAL_SERVER_ERROR"
		if appErr != nil {
			if err.Extensions == nil {
				err.Extensions = make(map[string]interface{})
			}
			err.Extensions["code"] = appErr.Code
			err.Message = appErr.ClientMessage()
			errorCode = appErr.Code
		}

		oc := graphql.GetOperationContext(ctx)
		opName := "anonymous"
		if oc != nil {
			if oc.OperationName != "" {
				opName = oc.OperationName
			} else if oc.Operation != nil && oc.Operation.Name != "" {
				opName = oc.Operation.Name
			}
		}

		var userID, role string
		if userCtx, ok := auth.UserFromContext(ctx); ok && userCtx != nil {
			userID = userCtx.UserID
			role = userCtx.Role
		}

		logger.FromContext(ctx).Warn("GraphQL operation error",
			"operation_name", opName,
			"error_code", errorCode,
			"request_id", middleware.GetRequestID(ctx),
			"user_id", userID,
			"role", role,
			"error", e.Error(),
		)

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
