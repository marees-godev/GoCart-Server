package client

import (
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
)

// TranslateGRPCError translates a gRPC status error into an application-specific AppError.
// It delegates to pkg/grpcclient.TranslateGRPCError to ensure unified error mapping across services.
func TranslateGRPCError(err error) error {
	return grpcclient.TranslateGRPCError(err)
}
