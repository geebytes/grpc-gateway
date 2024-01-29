package runtime

import (
	"context"

	"google.golang.org/grpc"
)

type Endpoint interface {
	RegisterService(serviceName string, ctx context.Context, mux *ServeMux, endpoint string, opts []grpc.DialOption) (err error)
}
