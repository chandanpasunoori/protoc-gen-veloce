package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// generateIndexFile emits a barrel re-exporting the runtime modules and every requested file.
// It fails when two modules export the same name, since `export *` would silently drop it.
func generateIndexFile(gen *protogen.Plugin, runtimes []runtimeModule, modules []*module) error {
	owner := map[string]string{}
	var conflicts []string
	claim := func(name, from string) {
		if prev, ok := owner[name]; ok {
			conflicts = append(conflicts, fmt.Sprintf("%q (%s, %s)", name, prev, from))
			return
		}
		owner[name] = from
	}

	var lines []string
	for _, r := range runtimes {
		for _, name := range r.exports {
			claim(name, r.prefix+".ts")
		}
		lines = append(lines, `export * from "`+relativeImportPath(indexFilePrefix, r.prefix)+`";`)
	}
	for _, m := range modules {
		if !m.file.Generate {
			continue
		}
		for _, name := range m.exports {
			claim(name, m.prefix+".ts")
		}
		lines = append(lines, `export * from "`+relativeImportPath(indexFilePrefix, m.prefix)+`";`)
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("index: names exported by more than one module: %s; rename the proto types or set index=false", strings.Join(conflicts, ", "))
	}

	g := gen.NewGeneratedFile(indexFilePrefix+".ts", "")
	g.P(generatedHeader)
	g.P()
	for _, line := range lines {
		g.P(line)
	}
	return nil
}
