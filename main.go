package main

import (
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

		var files []*protogen.File
		needsRuntime := false
		for _, f := range gen.Files {
			if !f.Generate && !strings.HasPrefix(f.Desc.Path(), "google/protobuf/") {
				continue
			}
			files = append(files, f)
			if len(f.Services) > 0 {
				needsRuntime = true
			}
		}

		if needsRuntime {
			generateRuntimeFile(gen)
		}

		for _, f := range files {
			generateFile(gen, f)
		}
		return nil
	})
}
