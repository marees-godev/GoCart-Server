package graphql

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/graphql-go/graphql"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	pkgGraphQL "github.com/marees-godev/GoCart-Server/pkg/graphql"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
)

type GraphQLRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

type Handler struct {
	schema graphql.Schema
	cfg    *config.Config
}

func NewHandler(schema graphql.Schema, cfg *config.Config) *Handler {
	return &Handler{
		schema: schema,
		cfg:    cfg,
	}
}

func (h *Handler) HandleQuery(c *fiber.Ctx) error {
	ctx := c.UserContext()
	if ctx == nil {
		ctx = context.Background()
	}

	var req GraphQLRequest
	if err := c.BodyParser(&req); err != nil {
		appErr := appErrors.BadRequest("invalid GraphQL request body")
		gqlErr := pkgGraphQL.FormatError(ctx, appErr)
		return c.Status(http.StatusBadRequest).JSON(pkgGraphQL.GraphQLErrorResponse{
			Errors: []pkgGraphQL.GraphQLError{gqlErr},
		})
	}

	if strings.TrimSpace(req.Query) == "" {
		appErr := appErrors.BadRequest("GraphQL query must not be empty")
		gqlErr := pkgGraphQL.FormatError(ctx, appErr)
		return c.Status(http.StatusBadRequest).JSON(pkgGraphQL.GraphQLErrorResponse{
			Errors: []pkgGraphQL.GraphQLError{gqlErr},
		})
	}

	// Introspection check
	if !h.cfg.GraphQLIntrospectionEnabled {
		if strings.Contains(req.Query, "__schema") || strings.Contains(req.Query, "__type") {
			appErr := appErrors.Forbidden("GraphQL introspection is disabled")
			gqlErr := pkgGraphQL.FormatError(ctx, appErr)
			return c.Status(http.StatusOK).JSON(pkgGraphQL.GraphQLErrorResponse{
				Errors: []pkgGraphQL.GraphQLError{gqlErr},
			})
		}
	}

	result := graphql.Do(graphql.Params{
		Schema:         h.schema,
		RequestString:  req.Query,
		OperationName:  req.OperationName,
		VariableValues: req.Variables,
		Context:        ctx,
	})

	if len(result.Errors) > 0 {
		formattedErrors := make([]pkgGraphQL.GraphQLError, 0, len(result.Errors))
		for _, err := range result.Errors {
			gqlErr := pkgGraphQL.FormatError(ctx, err.OriginalError())
			if err.OriginalError() == nil {
				gqlErr.Message = err.Message
			}
			if len(err.Locations) > 0 {
				gqlErr.Locations = make([]pkgGraphQL.Location, len(err.Locations))
				for i, loc := range err.Locations {
					gqlErr.Locations[i] = pkgGraphQL.Location{
						Line:   loc.Line,
						Column: loc.Column,
					}
				}
			}
			formattedErrors = append(formattedErrors, gqlErr)
		}

		responseMap := map[string]interface{}{
			"data":   result.Data,
			"errors": formattedErrors,
		}
		return c.Status(http.StatusOK).JSON(responseMap)
	}

	return c.Status(http.StatusOK).JSON(result)
}

func (h *Handler) HandlePlayground(c *fiber.Ctx) error {
	if !h.cfg.GraphQLIntrospectionEnabled {
		return c.Status(http.StatusForbidden).SendString("GraphQL Introspection/Playground is disabled")
	}

	html := `<!DOCTYPE html>
<html>
  <head>
    <title>GoCart GraphQL Playground</title>
    <link rel="stylesheet" href="https://unpkg.com/graphiql@3.0.6/graphiql.min.css" />
  </head>
  <body style="margin: 0; overflow: hidden;">
    <div id="graphiql" style="height: 100vh;"></div>
    <script src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
    <script src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
    <script src="https://unpkg.com/graphiql@3.0.6/graphiql.min.js"></script>
    <script>
      const fetcher = GraphiQL.createFetcher({ url: '/query' });
      ReactDOM.render(
        React.createElement(GraphiQL, { fetcher: fetcher }),
        document.getElementById('graphiql'),
      );
    </script>
  </body>
</html>`

	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.Status(http.StatusOK).SendString(html)
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Post("/query", h.HandleQuery)
	app.Get("/query", h.HandlePlayground)
	app.Get("/playground", h.HandlePlayground)
}

func MarshalResult(res *graphql.Result) ([]byte, error) {
	return json.Marshal(res)
}

