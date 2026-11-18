package main

import (
	"fmt"
	"strconv"
)

// FieldPresence controls how message fields are emitted as optional or required TypeScript properties.
type FieldPresence int

const (
	PresenceSplit FieldPresence = iota
	PresenceOptional
	PresenceRequired
)

type options struct {
	fieldPresence     FieldPresence
	labelOptionName   string
	importExtension   string
	index             bool
	debugMethodHeader bool
}

var opts = options{
	fieldPresence:   PresenceSplit,
	labelOptionName: "label",
	importExtension: ".js",
}

// setParam applies a single `key=value` plugin parameter.
func setParam(key, value string) error {
	switch key {
	case "field_presence":
		switch value {
		case "split":
			opts.fieldPresence = PresenceSplit
		case "optional":
			opts.fieldPresence = PresenceOptional
		case "required":
			opts.fieldPresence = PresenceRequired
		default:
			return fmt.Errorf("field_presence must be split, optional or required (got %q)", value)
		}
	case "label_option":
		if value == "" {
			return fmt.Errorf("label_option must not be empty")
		}
		opts.labelOptionName = value
	case "import_extension":
		switch value {
		case "js", "ts":
			opts.importExtension = "." + value
		case "none":
			opts.importExtension = ""
		default:
			return fmt.Errorf("import_extension must be js, ts or none (got %q)", value)
		}
	case "index":
		b, err := parseBool(key, value)
		if err != nil {
			return err
		}
		opts.index = b
	case "debug_method_header":
		b, err := parseBool(key, value)
		if err != nil {
			return err
		}
		opts.debugMethodHeader = b
	default:
		return fmt.Errorf("unknown parameter %q", key)
	}
	return nil
}

// parseBool reads a boolean parameter, treating a bare key as true.
func parseBool(key, value string) (bool, error) {
	if value == "" {
		return true, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false (got %q)", key, value)
	}
	return b, nil
}
