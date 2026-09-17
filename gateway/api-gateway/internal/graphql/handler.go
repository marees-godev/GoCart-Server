package graphql

import (
	"net/http"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	gqlHandler "github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	pkgGraphQL "github.com/marees-godev/GoCart-Server/pkg/graphql"
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
	return &Handler{
		server: srv,
		cfg:    cfg,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.server.ServeHTTP(w, r)
}

func (h *Handler) HandleQuery(c *fiber.Ctx) error {
	ctx := c.UserContext()
	if ctx == nil {
		ctx = c.Context()
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

	return adaptor.HTTPHandler(h.server)(c)
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
