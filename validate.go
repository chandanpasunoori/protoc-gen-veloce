package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// fieldRules returns the buf.validate.field rules attached to a field, or nil.
func fieldRules(field *protogen.Field) *validate.FieldRules {
	if field.Desc.Options() == nil {
		return nil
	}
	rules, ok := proto.GetExtension(field.Desc.Options(), validate.E_Field).(*validate.FieldRules)
	if !ok || rules == nil {
		return nil
	}
	return rules
}

// messageRuleEntries renders one `jsonName: { ... }` entry per field that carries checkable rules.
func messageRuleEntries(msg *protogen.Message) []string {
	var entries []string
	for _, field := range msg.Fields {
		rules := fieldRules(field)
		if rules == nil {
			continue
		}
		if body := renderFieldRules(field, rules, fieldLabel(field)); body != "" {
			entries = append(entries, "  "+propName(field.Desc.JSONName())+": "+body+",")
		}
	}
	return entries
}

// generateMessageRules emits the `<Message>Rules` descriptor for a message.
func (m *module) generateMessageRules(msg *protogen.Message, entries []string) {
	name := msg.GoIdent.GoName
	m.P("export const ", name, "Rules: ", m.runtime["MessageRules"], "<", name, "> = {")
	for _, e := range entries {
		m.P(e)
	}
	m.P("};")
	m.P()
}

// renderFieldRules renders the rules for one value of a field as a TypeScript object literal.
// The field supplies the scalar kind and enum; for map keys and values it is the map entry's field.
func renderFieldRules(field *protogen.Field, rules *validate.FieldRules, label string) string {
	var checks []string
	if rules.GetRequired() {
		checks = append(checks, "required: true")
	}
	switch rules.GetIgnore() {
	case validate.Ignore_IGNORE_IF_ZERO_VALUE:
		checks = append(checks, `ignore: "IF_ZERO"`)
	case validate.Ignore_IGNORE_ALWAYS:
		checks = append(checks, `ignore: "ALWAYS"`)
	}
	if s := renderStringRules(rules.GetString()); s != "" {
		checks = append(checks, "string: "+s)
	}
	if n := renderNumberRules(rules); n != "" {
		checks = append(checks, "number: "+n)
	}
	if e := renderEnumRules(field, rules.GetEnum()); e != "" {
		checks = append(checks, "enum: "+e)
	}
	if b := renderBytesRules(rules.GetBytes()); b != "" {
		checks = append(checks, "bytes: "+b)
	}
	if r := renderRepeatedRules(field, rules.GetRepeated(), label); r != "" {
		checks = append(checks, "repeated: "+r)
	}
	if mr := renderMapRules(field, rules.GetMap(), label); mr != "" {
		checks = append(checks, "map: "+mr)
	}
	if len(checks) == 0 {
		return ""
	}
	return "{ label: " + jsString(label) + ", " + strings.Join(checks, ", ") + " }"
}

func renderStringRules(r *validate.StringRules) string {
	if r == nil {
		return ""
	}
	var p []string
	str := func(key string, set bool, v string) {
		if set {
			p = append(p, key+": "+jsString(v))
		}
	}
	num := func(key string, set bool, v uint64) {
		if set {
			p = append(p, key+": "+strconv.FormatUint(v, 10))
		}
	}
	str("const", r.Const != nil, r.GetConst())
	num("len", r.Len != nil, r.GetLen())
	num("minLen", r.MinLen != nil, r.GetMinLen())
	num("maxLen", r.MaxLen != nil, r.GetMaxLen())
	num("lenBytes", r.LenBytes != nil, r.GetLenBytes())
	num("minBytes", r.MinBytes != nil, r.GetMinBytes())
	num("maxBytes", r.MaxBytes != nil, r.GetMaxBytes())
	str("pattern", r.Pattern != nil, r.GetPattern())
	str("prefix", r.Prefix != nil, r.GetPrefix())
	str("suffix", r.Suffix != nil, r.GetSuffix())
	str("contains", r.Contains != nil, r.GetContains())
	str("notContains", r.NotContains != nil, r.GetNotContains())
	if len(r.GetIn()) > 0 {
		p = append(p, "in: "+stringArray(r.GetIn()))
	}
	if len(r.GetNotIn()) > 0 {
		p = append(p, "notIn: "+stringArray(r.GetNotIn()))
	}
	str("format", stringFormat(r) != "", stringFormat(r))
	switch r.GetWellKnownRegex() {
	case validate.KnownRegex_KNOWN_REGEX_HTTP_HEADER_NAME:
		p = append(p, `wellKnownRegex: "httpHeaderName"`)
	case validate.KnownRegex_KNOWN_REGEX_HTTP_HEADER_VALUE:
		p = append(p, `wellKnownRegex: "httpHeaderValue"`)
	}
	return objectLiteral(p)
}

// stringFormat maps protovalidate's well-known string predicates to a single format tag.
func stringFormat(r *validate.StringRules) string {
	switch {
	case r.GetEmail():
		return "email"
	case r.GetHostname():
		return "hostname"
	case r.GetIp():
		return "ip"
	case r.GetIpv4():
		return "ipv4"
	case r.GetIpv6():
		return "ipv6"
	case r.GetUri():
		return "uri"
	case r.GetUriRef():
		return "uriRef"
	case r.GetAddress():
		return "address"
	case r.GetUuid():
		return "uuid"
	case r.GetTuuid():
		return "tuuid"
	case r.GetHostAndPort():
		return "hostAndPort"
	case r.GetUlid():
		return "ulid"
	}
	return ""
}

var (
	int32RuleKinds = map[string]bool{"int32": true, "uint32": true, "sint32": true, "fixed32": true, "sfixed32": true}
	int64RuleKinds = map[string]bool{"int64": true, "uint64": true, "sint64": true, "fixed64": true, "sfixed64": true}
	floatRuleKinds = map[string]bool{"float": true, "double": true}
)

// renderNumberRules renders comparison rules for any numeric scalar kind, reading the populated
// FieldRules `type` oneof reflectively so every width is handled uniformly. 64-bit bounds are
// emitted as bigint literals because their JSON values are strings that may exceed 2^53.
func renderNumberRules(rules *validate.FieldRules) string {
	sub, kind := populatedRuleSet(rules)
	if sub == nil || !(int32RuleKinds[kind] || int64RuleKinds[kind] || floatRuleKinds[kind]) {
		return ""
	}

	var p []string
	sub.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch name := string(fd.Name()); name {
		case "const", "lt", "lte", "gt", "gte":
			p = append(p, name+": "+numberLit(kind, v))
		case "in", "not_in":
			key := "in"
			if name == "not_in" {
				key = "notIn"
			}
			list := v.List()
			items := make([]string, 0, list.Len())
			for i := 0; i < list.Len(); i++ {
				items = append(items, numberLit(kind, list.Get(i)))
			}
			p = append(p, key+": ["+strings.Join(items, ", ")+"]")
		case "finite":
			if v.Bool() {
				p = append(p, "finite: true")
			}
		}
		return true
	})
	return objectLiteral(p)
}

// populatedRuleSet returns the message set on the FieldRules `type` oneof along with its name.
func populatedRuleSet(rules *validate.FieldRules) (protoreflect.Message, string) {
	m := rules.ProtoReflect()
	od := m.Descriptor().Oneofs().ByName("type")
	if od == nil {
		return nil, ""
	}
	fd := m.WhichOneof(od)
	if fd == nil {
		return nil, ""
	}
	return m.Get(fd).Message(), string(fd.Name())
}

// numberLit renders a numeric rule bound as a TypeScript literal for the given rule kind.
func numberLit(kind string, v protoreflect.Value) string {
	switch {
	case int64RuleKinds[kind]:
		return fmt.Sprintf("%vn", v.Interface())
	case floatRuleKinds[kind]:
		bits := 64
		if kind == "float" {
			bits = 32
		}
		f := v.Float()
		switch {
		case math.IsNaN(f):
			return "Number.NaN"
		case math.IsInf(f, 1):
			return "Number.POSITIVE_INFINITY"
		case math.IsInf(f, -1):
			return "Number.NEGATIVE_INFINITY"
		}
		return strconv.FormatFloat(f, 'g', -1, bits)
	}
	return fmt.Sprintf("%v", v.Interface())
}

func renderEnumRules(field *protogen.Field, r *validate.EnumRules) string {
	if r == nil {
		return ""
	}
	var p []string
	if r.Const != nil {
		p = append(p, fmt.Sprintf("const: %d", r.GetConst()))
	}
	if r.GetDefinedOnly() {
		p = append(p, "definedOnly: true")
	}
	if len(r.GetIn()) > 0 {
		p = append(p, "in: "+intArray(r.GetIn()))
	}
	if len(r.GetNotIn()) > 0 {
		p = append(p, "notIn: "+intArray(r.GetNotIn()))
	}
	if len(p) == 0 {
		return ""
	}
	if field.Enum != nil {
		names := make([]string, 0, len(field.Enum.Values))
		numbers := make([]string, 0, len(field.Enum.Values))
		for _, v := range field.Enum.Values {
			names = append(names, jsString(string(v.Desc.Name())))
			numbers = append(numbers, fmt.Sprintf("%d", int32(v.Desc.Number())))
		}
		p = append(p, "values: ["+strings.Join(names, ", ")+"]", "numbers: ["+strings.Join(numbers, ", ")+"]")
	}
	return objectLiteral(p)
}

func renderBytesRules(r *validate.BytesRules) string {
	if r == nil {
		return ""
	}
	var p []string
	if r.Len != nil {
		p = append(p, "len: "+strconv.FormatUint(r.GetLen(), 10))
	}
	if r.MinLen != nil {
		p = append(p, "minLen: "+strconv.FormatUint(r.GetMinLen(), 10))
	}
	if r.MaxLen != nil {
		p = append(p, "maxLen: "+strconv.FormatUint(r.GetMaxLen(), 10))
	}
	return objectLiteral(p)
}

func renderRepeatedRules(field *protogen.Field, r *validate.RepeatedRules, label string) string {
	if r == nil {
		return ""
	}
	var p []string
	if r.MinItems != nil {
		p = append(p, "minItems: "+strconv.FormatUint(r.GetMinItems(), 10))
	}
	if r.MaxItems != nil {
		p = append(p, "maxItems: "+strconv.FormatUint(r.GetMaxItems(), 10))
	}
	if r.GetUnique() {
		p = append(p, "unique: true")
	}
	if items := r.GetItems(); items != nil {
		if inner := renderFieldRules(field, items, label); inner != "" {
			p = append(p, "items: "+inner)
		}
	}
	return objectLiteral(p)
}

func renderMapRules(field *protogen.Field, r *validate.MapRules, label string) string {
	if r == nil || !field.Desc.IsMap() {
		return ""
	}
	key, value := field.Message.Fields[0], field.Message.Fields[1]
	var p []string
	if r.MinPairs != nil {
		p = append(p, "minPairs: "+strconv.FormatUint(r.GetMinPairs(), 10))
	}
	if r.MaxPairs != nil {
		p = append(p, "maxPairs: "+strconv.FormatUint(r.GetMaxPairs(), 10))
	}
	if keys := r.GetKeys(); keys != nil {
		if inner := renderFieldRules(key, keys, label); inner != "" {
			p = append(p, "keys: "+inner)
		}
	}
	if values := r.GetValues(); values != nil {
		if inner := renderFieldRules(value, values, label); inner != "" {
			p = append(p, "values: "+inner)
		}
	}
	return objectLiteral(p)
}

// fieldLabel resolves the human-readable field name used in violation messages.
func fieldLabel(field *protogen.Field) string {
	if explicit := explicitLabel(string(field.Comments.Leading)); explicit != "" {
		return explicit
	}
	return titleCaseSnake(strings.ToUpper(string(field.Desc.Name())))
}

func objectLiteral(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func stringArray(vals []string) string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, jsString(v))
	}
	return "[" + strings.Join(out, ", ") + "]"
}

func intArray(vals []int32) string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, strconv.FormatInt(int64(v), 10))
	}
	return "[" + strings.Join(out, ", ") + "]"
}
