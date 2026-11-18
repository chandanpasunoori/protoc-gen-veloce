package main

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	labelMarker      = "label:"
	enumValueOptions = "google.protobuf.EnumValueOptions"
	nullValueEnum    = "google.protobuf.NullValue"
)

var (
	labelExtension protoreflect.ExtensionType
	labelResolver  *protoregistry.Types
)

// resolveLabelExtension locates the string extension on EnumValueOptions named by the
// label_option parameter among the input files, so labels can come from the proto itself.
func resolveLabelExtension(gen *protogen.Plugin) {
	for _, f := range gen.Files {
		for _, ext := range f.Extensions {
			d := ext.Desc
			if string(d.Name()) != opts.labelOptionName || d.Kind() != protoreflect.StringKind {
				continue
			}
			if d.ContainingMessage() == nil || d.ContainingMessage().FullName() != enumValueOptions {
				continue
			}
			xt := dynamicpb.NewExtensionType(d)
			types := new(protoregistry.Types)
			if err := types.RegisterExtension(xt); err != nil {
				return
			}
			labelExtension, labelResolver = xt, types
			return
		}
	}
}

// labelFromOption reads the label option off an enum value. Custom extensions arrive as unknown
// fields in plugin input, so the options are re-decoded against the resolved extension and the
// field is matched by name, since a dynamic extension type has no identity to look up.
func labelFromOption(v *protogen.EnumValue) string {
	if labelExtension == nil || v.Desc.Options() == nil {
		return ""
	}
	raw, err := proto.Marshal(v.Desc.Options())
	if err != nil {
		return ""
	}
	resolved := &descriptorpb.EnumValueOptions{}
	if err := (proto.UnmarshalOptions{Resolver: labelResolver}).Unmarshal(raw, resolved); err != nil {
		return ""
	}
	want := labelExtension.TypeDescriptor().FullName()
	label := ""
	resolved.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		if fd.FullName() == want {
			label = strings.TrimSpace(val.String())
			return false
		}
		return true
	})
	return label
}

// enumNames lists the top-level identifiers an enum generates: exported ones, then internal ones.
func enumNames(enum *protogen.Enum) (exported, internal []string) {
	name := enum.GoIdent.GoName
	if enum.Desc.FullName() == nullValueEnum {
		return []string{name}, nil
	}
	return []string{name, name + "Number", name + "Label", name + "Values", name + "Options", "is" + name, "to" + name},
		[]string{name + "ByNumber"}
}

// generateEnum emits a const-object enum keyed by value name, its value type, and lookup helpers
// for numbers, labels, ordered values, select options, a type guard and wire-form coercion.
func (m *module) generateEnum(enum *protogen.Enum) {
	name := enum.GoIdent.GoName
	if enum.Desc.FullName() == nullValueEnum {
		m.P("export type ", name, " = null;")
		m.P()
		return
	}
	prefix := screamingSnake(string(enum.Desc.Name())) + "_"

	m.P("export const ", name, " = {")
	for _, v := range enum.Values {
		vn := string(v.Desc.Name())
		m.P("  ", vn, ": \"", vn, "\",")
	}
	m.P("} as const;")
	m.P("export type ", name, " = (typeof ", name, ")[keyof typeof ", name, "];")
	m.P()

	m.P("export const ", name, "Number: { readonly [K in ", name, "]: number } = {")
	for _, v := range enum.Values {
		m.P("  ", string(v.Desc.Name()), ": ", int32(v.Desc.Number()), ",")
	}
	m.P("};")
	m.P()

	m.P("export const ", name, "Label: { readonly [K in ", name, "]: string } = {")
	for _, v := range enum.Values {
		m.P("  ", string(v.Desc.Name()), ": ", jsString(enumValueLabel(v, prefix)), ",")
	}
	m.P("};")
	m.P()

	m.P("export const ", name, "Values: readonly ", name, "[] = [")
	for _, v := range enum.Values {
		m.P("  ", name, ".", string(v.Desc.Name()), ",")
	}
	m.P("];")
	m.P()

	m.P("export const ", name, "Options: readonly { readonly value: ", name, "; readonly label: string }[] = [")
	for _, v := range enum.Values {
		if v.Desc.Number() == 0 {
			continue
		}
		vn := string(v.Desc.Name())
		m.P("  { value: ", name, ".", vn, ", label: ", name, "Label.", vn, " },")
	}
	m.P("];")
	m.P()

	m.P("const ", name, "ByNumber: { readonly [n: number]: ", name, " | undefined } = {")
	seen := map[int32]bool{}
	for _, v := range enum.Values {
		n := int32(v.Desc.Number())
		if seen[n] {
			continue
		}
		seen[n] = true
		m.P("  ", jsString(fmt.Sprint(n)), ": ", name, ".", string(v.Desc.Name()), ",")
	}
	m.P("};")
	m.P()

	m.P("export function is", name, "(value: unknown): value is ", name, " {")
	m.P("  return typeof value === \"string\" && Object.hasOwn(", name, ", value);")
	m.P("}")
	m.P()

	m.P("export function to", name, "(value: unknown): ", name, " | undefined {")
	m.P("  if (typeof value === \"number\") return ", name, "ByNumber[value];")
	m.P("  return is", name, "(value) ? value : undefined;")
	m.P("}")
	m.P()
}

// enumValueLabel resolves the display label for an enum value: the label option wins, then a
// `label:` line in the leading comment, then the value name with the enum prefix stripped.
func enumValueLabel(v *protogen.EnumValue, prefix string) string {
	if explicit := labelFromOption(v); explicit != "" {
		return explicit
	}
	if explicit := explicitLabel(string(v.Comments.Leading)); explicit != "" {
		return explicit
	}
	return titleCaseSnake(strings.TrimPrefix(string(v.Desc.Name()), prefix))
}

// explicitLabel extracts the value of a `label:` directive from a leading comment block.
func explicitLabel(comment string) string {
	for _, line := range strings.Split(comment, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), labelMarker) {
			return strings.TrimSpace(line[len(labelMarker):])
		}
	}
	return ""
}

// screamingSnake converts a CamelCase identifier to SCREAMING_SNAKE_CASE.
func screamingSnake(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prevLower := unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1])
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevLower || (unicode.IsUpper(runes[i-1]) && nextLower) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// titleCaseSnake converts SCREAMING_SNAKE_CASE to a space-separated Title Case label.
func titleCaseSnake(s string) string {
	parts := strings.Split(s, "_")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		lower := strings.ToLower(p)
		out = append(out, strings.ToUpper(lower[:1])+lower[1:])
	}
	return strings.Join(out, " ")
}
