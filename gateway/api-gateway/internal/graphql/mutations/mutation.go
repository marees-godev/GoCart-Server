package mutation

import (
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
)

type MutationResolver struct {
	Clients *grpc.Clients
}

func NewMutationResolver(clients *grpc.Clients) *MutationResolver {
	return &MutationResolver{
		Clients: clients,
	}
}
