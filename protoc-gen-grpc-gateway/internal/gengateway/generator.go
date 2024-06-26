package gengateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"path"

	"github.com/grpc-ecosystem/grpc-gateway/v2/internal/descriptor"
	gen "github.com/grpc-ecosystem/grpc-gateway/v2/internal/generator"
	"github.com/grpc-ecosystem/grpc-gateway/v2/internal/httprule"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

var errNoTargetService = errors.New("no target service defined in the file")

type generator struct {
	reg                *descriptor.Registry
	baseImports        []descriptor.GoPackage
	useRequestContext  bool
	registerFuncSuffix string
	allowPatchFeature  bool
	standalone         bool
}

// New returns a new generator which generates grpc gateway files.
func New(reg *descriptor.Registry, useRequestContext bool, registerFuncSuffix string,
	allowPatchFeature, standalone bool) gen.Generator {
	var imports []descriptor.GoPackage
	for _, pkgpath := range []string{
		"context",
		"io",
		"net/http",
		"github.com/grpc-ecosystem/grpc-gateway/v2/runtime",
		"github.com/grpc-ecosystem/grpc-gateway/v2/utilities",
		"google.golang.org/protobuf/proto",
		"google.golang.org/grpc",
		"google.golang.org/grpc/codes",
		"google.golang.org/grpc/grpclog",
		"google.golang.org/grpc/metadata",
		"google.golang.org/grpc/status",
	} {
		pkg := descriptor.GoPackage{
			Path: pkgpath,
			Name: path.Base(pkgpath),
		}
		if err := reg.ReserveGoPackageAlias(pkg.Name, pkg.Path); err != nil {
			for i := 0; ; i++ {
				alias := fmt.Sprintf("%s_%d", pkg.Name, i)
				if err := reg.ReserveGoPackageAlias(alias, pkg.Path); err != nil {
					continue
				}
				pkg.Alias = alias
				break
			}
		}
		imports = append(imports, pkg)
	}

	return &generator{
		reg:                reg,
		baseImports:        imports,
		useRequestContext:  useRequestContext,
		registerFuncSuffix: registerFuncSuffix,
		allowPatchFeature:  allowPatchFeature,
		standalone:         standalone,
	}
}
func (g *generator) generateEndpoint(services []*descriptor.Service) (*descriptor.ResponseFile, error) {
	if len(services) == 0 {
		return nil, errNoTargetService
	}

	code, err := applyEndpointTemplate(services, g.registerFuncSuffix)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source([]byte(code))
	if err != nil {
		grpclog.Errorf("%v: %s", err, code)
		return nil, err
	}

	// fmt.Println(string(formatted))
	return &descriptor.ResponseFile{
		GoPkg: services[0].File.GoPkg,
		CodeGeneratorResponse_File: &pluginpb.CodeGeneratorResponse_File{
			Name:    proto.String("gateway" + ".pb.gw.ep.go"),
			Content: proto.String(string(formatted)),
		},
	}, nil
}
func (g *generator) serviceEndpoints(services []*descriptor.Service) (*descriptor.ResponseFile, error) {
	type httpEndpointItem struct {
		Pattern        runtime.Pattern
		Template       httprule.Template
		HttpMethod     string
		FullMethodName string
		HttpUri        string
		PathParams     []string
		InName         string
		OutName        string
		IsClientStream bool
		IsServerStream bool
		Pkg            string
		InPkg          string
		OutPkg         string
	}

	binds := make(map[string][]*httpEndpointItem)
	for _, svc := range services {
		// for _,ext:=range svc.Options..GetExtension(){
		// 	fmt.Println(ext)

		// }
		for _, m := range svc.Methods {

			key := fmt.Sprintf("/%s.%s/%s", *svc.File.Package, svc.GetName(), m.GetName())
			items := make([]*httpEndpointItem, 0)
			for _, b := range m.Bindings {
				if b.PathTmpl.Template == "" {
					continue
				}
				item := &httpEndpointItem{}
				item.Template = b.PathTmpl
				item.HttpMethod = b.HTTPMethod
				item.FullMethodName = key
				item.HttpUri = b.PathTmpl.Template
				item.PathParams = make([]string, 0)
				item.InName = m.RequestType.GetName()
				item.OutName = m.ResponseType.GetName()
				item.IsClientStream = m.GetClientStreaming()
				item.IsServerStream = m.GetServerStreaming()
				item.Pkg = *svc.File.Package
				if m.RequestType != nil {
					item.InPkg = *m.RequestType.File.Package
				}
				if m.ResponseType != nil {
					item.OutPkg = *m.ResponseType.File.Package
				}
				for _, path := range b.PathParams {
					item.PathParams = append(item.PathParams, path.FieldPath.String())
				}
				items = append(items, item)
			}
			binds[key] = items

		}
	}
	jsonData, err := json.MarshalIndent(binds, "", "    ")
	if err != nil {
		return nil, err
	}
	// file, err := os.Create("binds.json")
	// if err != nil {
	// 	return fmt.Errorf("Error occurred during file creation. Error: %s", err.Error())
	// }
	// defer file.Close()
	// _, err = file.Write(jsonData)
	f := &descriptor.ResponseFile{
		GoPkg: services[0].File.GoPkg,
		CodeGeneratorResponse_File: &pluginpb.CodeGeneratorResponse_File{
			Name:    proto.String("gateway" + ".json"),
			Content: proto.String(string(jsonData)),
		},
	}
	return f, nil
}
func (g *generator) Generate(targets []*descriptor.File) ([]*descriptor.ResponseFile, error) {
	var files []*descriptor.ResponseFile
	var services []*descriptor.Service
	for _, file := range targets {
		if grpclog.V(1) {
			grpclog.Infof("Processing %s", file.GetName())
		}

		code, service, err := g.generate(file)
		if errors.Is(err, errNoTargetService) {
			if grpclog.V(1) {
				grpclog.Infof("%s: %v", file.GetName(), err)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source([]byte(code))
		if err != nil {
			grpclog.Errorf("%v: %s", err, code)
			return nil, err
		}
		files = append(files, &descriptor.ResponseFile{
			GoPkg: file.GoPkg,
			CodeGeneratorResponse_File: &pluginpb.CodeGeneratorResponse_File{
				Name:    proto.String(file.GeneratedFilenamePrefix + ".pb.gw.go"),
				Content: proto.String(string(formatted)),
			},
		})
		services = append(services, service...)
	}
	if len(services) > 0 {
		// endpoint, err := g.generateEndpoint(services)
		// if err != nil {
		// 	return nil, err
		// }
		// files = append(files, endpoint)
		f, err := g.serviceEndpoints(services)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

func (g *generator) generate(file *descriptor.File) (string, []*descriptor.Service, error) {
	pkgSeen := make(map[string]bool)
	var imports []descriptor.GoPackage
	for _, pkg := range g.baseImports {
		pkgSeen[pkg.Path] = true
		imports = append(imports, pkg)
	}

	if g.standalone {
		imports = append(imports, file.GoPkg)
	}

	for _, svc := range file.Services {
		for _, m := range svc.Methods {
			imports = append(imports, g.addEnumPathParamImports(file, m, pkgSeen)...)
			pkg := m.RequestType.File.GoPkg
			if len(m.Bindings) == 0 ||
				pkg == file.GoPkg || pkgSeen[pkg.Path] {
				continue
			}
			pkgSeen[pkg.Path] = true
			imports = append(imports, pkg)
		}
	}
	params := param{
		File:               file,
		Imports:            imports,
		UseRequestContext:  g.useRequestContext,
		RegisterFuncSuffix: g.registerFuncSuffix,
		AllowPatchFeature:  g.allowPatchFeature,
	}
	if g.reg != nil {
		params.OmitPackageDoc = g.reg.GetOmitPackageDoc()
	}
	return applyTemplate(params, g.reg)
}

// addEnumPathParamImports handles adding import of enum path parameter go packages
func (g *generator) addEnumPathParamImports(file *descriptor.File, m *descriptor.Method, pkgSeen map[string]bool) []descriptor.GoPackage {
	var imports []descriptor.GoPackage
	for _, b := range m.Bindings {
		for _, p := range b.PathParams {
			e, err := g.reg.LookupEnum("", p.Target.GetTypeName())
			if err != nil {
				continue
			}
			pkg := e.File.GoPkg
			if pkg == file.GoPkg || pkgSeen[pkg.Path] {
				continue
			}
			pkgSeen[pkg.Path] = true
			imports = append(imports, pkg)
		}
	}
	return imports
}
