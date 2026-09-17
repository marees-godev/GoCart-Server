package graphql

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	resolvers "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
)

type Server struct {
	resolver *resolvers.Resolver
	handler  *Handler
}

func NewServer(r *resolvers.Resolver) *Server {
	es, _ := NewSchema(r)
	h := NewHandler(es, &config.Config{GraphQLIntrospectionEnabled: true})
	return &Server{
		resolver: r,
		handler:  h,
	}
}

func PlaygroundHandler(title, endpoint string) http.Handler {
	return playground.Handler(title, endpoint)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.server.ServeHTTP(w, r)
}
