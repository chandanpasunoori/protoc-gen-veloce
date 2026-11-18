package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-veloce: %v\n", err)
		os.Exit(1)
	}
}

// run reads a CodeGeneratorRequest, generates TypeScript and writes the CodeGeneratorResponse.
func run(in io.Reader, out io.Writer) error {
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		return err
	}
	ensureGoPackages(req)

	gen, err := protogen.Options{ParamFunc: setParam}.New(req)
	if err != nil {
		return err
	}
	gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
	if err := generate(gen); err != nil {
		gen.Error(err)
	}

	resp, err := proto.Marshal(gen.Response())
	if err != nil {
		return err
	}
	_, err = out.Write(resp)
	return err
}

// ensureGoPackages gives every file a placeholder go_package, since protogen insists on one
// and this plugin never emits Go.
func ensureGoPackages(req *pluginpb.CodeGeneratorRequest) {
	for _, f := range req.GetProtoFile() {
		if f.GetOptions().GetGoPackage() != "" {
			continue
		}
		if f.Options == nil {
			f.Options = &descriptorpb.FileOptions{}
		}
		f.Options.GoPackage = proto.String("veloce.invalid/" + strings.TrimSuffix(f.GetName(), ".proto"))
	}
}

// generate emits one module per file that is requested or whose types a requested file
// references, followed by the runtime modules those files need.
func generate(gen *protogen.Plugin) error {
	resolveLabelExtension(gen)
	classifyMessages(gen)

	var modules []*module
	for _, f := range filesToGenerate(gen) {
		m, err := generateFile(gen, f)
		if err != nil {
			return err
		}
		modules = append(modules, m)
	}

	var needsClient, needsValidate, needsWkt bool
	for _, m := range modules {
		needsClient = needsClient || m.hasRPCs
		needsValidate = needsValidate || m.hasRules
		needsWkt = needsWkt || wktHelperFiles[m.file.Desc.Path()]
	}

	var runtimes []runtimeModule
	if needsClient {
		runtimes = append(runtimes, emitRuntime(gen, runtimeFilePrefix, runtimeClientSource))
	}
	if needsWkt {
		runtimes = append(runtimes, emitRuntime(gen, wktFilePrefix, wktRuntimeSource))
	}
	if needsValidate {
		runtimes = append(runtimes, emitRuntime(gen, validateFilePrefix, validateRuntimeSource))
	}

	if opts.index {
		return generateIndexFile(gen, runtimes, modules)
	}
	return nil
}

// filesToGenerate returns the requested files plus every dependency defining a type that is
// reachable from them, in request order.
func filesToGenerate(gen *protogen.Plugin) []*protogen.File {
	needed := map[string]bool{}
	seen := map[string]bool{}

	var visitMessage func(msg *protogen.Message)
	var visitField func(field *protogen.Field)
	visitEnum := func(enum *protogen.Enum) {
		needed[enum.Desc.ParentFile().Path()] = true
	}
	visitField = func(field *protogen.Field) {
		if field.Desc.IsMap() {
			visitField(field.Message.Fields[1])
			return
		}
		if field.Enum != nil {
			visitEnum(field.Enum)
		}
		if field.Message != nil {
			visitMessage(field.Message)
		}
	}
	visitMessage = func(msg *protogen.Message) {
		name := string(msg.Desc.FullName())
		if seen[name] {
			return
		}
		seen[name] = true
		needed[msg.Desc.ParentFile().Path()] = true
		for _, field := range msg.Fields {
			visitField(field)
		}
		for _, nested := range msg.Messages {
			if !nested.Desc.IsMapEntry() {
				visitMessage(nested)
			}
		}
		for _, enum := range msg.Enums {
			visitEnum(enum)
		}
	}

	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}
		needed[f.Desc.Path()] = true
		for _, msg := range f.Messages {
			visitMessage(msg)
		}
		for _, svc := range f.Services {
			for _, m := range svc.Methods {
				if !isSupportedMethod(m) {
					continue
				}
				visitMessage(m.Input)
				if !isHTTPBody(m.Output) {
					visitMessage(m.Output)
				}
			}
		}
	}

	var files []*protogen.File
	for _, f := range gen.Files {
		if needed[f.Desc.Path()] {
			files = append(files, f)
		}
	}
	return files
}

// tsPrefix is the output path of a file's module, without extension, mirroring the proto path.
func tsPrefix(f *protogen.File) string {
	return strings.TrimSuffix(f.Desc.Path(), ".proto")
}
