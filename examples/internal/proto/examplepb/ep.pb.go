package examplepb

import (
	"context"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

type endpointRegisters struct {
	serviceRegisters map[string]func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error)
}

func NewEndpointRegisters() *endpointRegisters {
	services := map[string]func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error){

		// "ExampleService": RegisterExampleServiceFromEndpoint,
	}
	return &endpointRegisters{
		serviceRegisters: services,
	}
}
