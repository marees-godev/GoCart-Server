package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/generated"
	resolvers "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	pkgGraphQL "github.com/marees-godev/GoCart-Server/pkg/graphql"
	"github.com/marees-godev/GoCart-Server/pkg/middleware"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type Server struct {
	srv      *handler.Server
	resolver *resolvers.Resolver
}

func NewServer(r *resolvers.Resolver) *Server {
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: r}))

	srv.SetErrorPresenter(func(ctx context.Context, e error) *gqlerror.Error {
		err := graphql.DefaultErrorPresenter(ctx, e)
		var appErr *appErrors.AppError
		code := appErrors.CodeInternalError
		if errors.As(e, &appErr) {
			code = appErr.Code
		}
		if err.Extensions == nil {
			err.Extensions = make(map[string]any)
		}
		err.Extensions["code"] = code
		if reqID := middleware.GetRequestID(ctx); reqID != "" {
			err.Extensions["request_id"] = reqID
		}
		if traceID := pkgGraphQL.GetTraceID(ctx); traceID != "" {
			err.Extensions["trace_id"] = traceID
		}
		return err
	})

	return &Server{
		srv:      srv,
		resolver: r,
	}
}

func PlaygroundHandler(title, endpoint string) http.Handler {
	return playground.Handler(title, endpoint)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Query().Get("query") == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "GraphQL server is running",
		})
		return
	}

	s.srv.ServeHTTP(w, r)
}
