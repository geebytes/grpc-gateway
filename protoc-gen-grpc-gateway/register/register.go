package register

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	desc "github.com/grpc-ecosystem/grpc-gateway/v2/internal/descriptor"
	"github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway/internal/gengateway"
)

func Register(fds *descriptor.FileDescriptorSet, standalone bool, generatedFilenamePrefix string) ([]string, error) {
	reg := desc.NewRegistry()
	reg.SetStandalone(standalone)
	targets, err := reg.FromFileDescriptorSet(fds, generatedFilenamePrefix)
	if err != nil {
		return nil, err
	}
	generator := gengateway.New(reg, true, "Handler", true, standalone)
	unboundHTTPRules := reg.UnboundExternalHTTPRules()
	if len(unboundHTTPRules) != 0 {
		return nil, fmt.Errorf("HTTP rules without a matching selector: %s", strings.Join(unboundHTTPRules, ", "))
	}
	gatewayContent := make([]string, 0)
	files, err := generator.Generate(targets)
	if err != nil {
		return nil, err

	}
	for _, f := range files {
		if !strings.HasSuffix(f.GetName(), ".json") {
			continue
		}
		if f.GetContent() != "" {
			gatewayContent = append(gatewayContent, f.GetContent())
		}
	}
	if len(gatewayContent) == 0 {
		return nil, fmt.Errorf("no gateway content")
	}

	return gatewayContent, nil
}
