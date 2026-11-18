package main

import (
	"regexp"

	"google.golang.org/protobuf/compiler/protogen"
)

const (
	runtimeFilePrefix  = "veloce_client"
	wktFilePrefix      = "veloce_wkt"
	validateFilePrefix = "veloce_validate"
	indexFilePrefix    = "index"
)

var wktHelperFiles = map[string]bool{
	"google/protobuf/timestamp.proto":  true,
	"google/protobuf/duration.proto":   true,
	"google/protobuf/field_mask.proto": true,
}

type runtimeModule struct {
	prefix  string
	exports []string
}

var exportPattern = regexp.MustCompile(`(?m)^export (?:declare )?(?:abstract )?(?:async )?(?:function\*?|const|let|class|interface|type) (\w+)`)

// emitRuntime writes one of the fixed runtime modules and records the names it exports.
func emitRuntime(gen *protogen.Plugin, prefix, source string) runtimeModule {
	g := gen.NewGeneratedFile(prefix+".ts", "")
	g.P(generatedHeader)
	g.P()
	g.P(source)
	var exports []string
	for _, match := range exportPattern.FindAllStringSubmatch(source, -1) {
		exports = append(exports, match[1])
	}
	return runtimeModule{prefix: prefix, exports: exports}
}
