package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/99designs/gqlgen/graphql/playground"
	resolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	pkgGraphQL "github.com/marees-godev/GoCart-Server/pkg/graphql"
)

type Request struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
}

type Response struct {
	Data   map[string]any            `json:"data,omitempty"`
	Errors []pkgGraphQL.GraphQLError `json:"errors,omitempty"`
}

type Server struct {
	resolver *resolver.Resolver
}

func NewServer(r *resolver.Resolver) *Server {
	return &Server{
		resolver: r,
	}
}

func PlaygroundHandler(title, endpoint string) http.Handler {
	return playground.Handler(title, endpoint)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "GraphQL server is running",
		})
		return
	}

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req Request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, err)
		return
	}
	defer r.Body.Close()

	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, err)
			return
		}
	}

	res := s.execute(r.Context(), req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) execute(ctx context.Context, req Request) Response {
	data := make(map[string]any)

	if req.Query != "" {
		if bytes.Contains([]byte(req.Query), []byte("health")) {
			healthVal, err := s.resolver.Health(ctx)
			if err != nil {
				return Response{Errors: []pkgGraphQL.GraphQLError{pkgGraphQL.FormatError(ctx, err)}}
			}
			data["health"] = healthVal
		}
		if bytes.Contains([]byte(req.Query), []byte("version")) {
			versionVal, err := s.resolver.GetVersion(ctx)
			if err != nil {
				return Response{Errors: []pkgGraphQL.GraphQLError{pkgGraphQL.FormatError(ctx, err)}}
			}
			data["version"] = versionVal
		}
	}

	if len(data) == 0 && req.Query != "" {
		data["health"] = "OK"
		data["version"] = s.resolver.Version
	}

	return Response{Data: data}
}

func writeErrorResponse(w http.ResponseWriter, r *http.Request, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	gqlErr := pkgGraphQL.FormatError(r.Context(), err)
	_ = json.NewEncoder(w).Encode(Response{
		Errors: []pkgGraphQL.GraphQLError{gqlErr},
	})
}
