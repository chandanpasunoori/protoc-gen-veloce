package main

import (
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var responseOnlyMessages = map[protoreflect.FullName]bool{}

// classifyMessages splits messages into request-reachable and response-reachable sets so
// request literals stay easy to build while response reads stay free of optional chaining.
//
// A message used in both positions -- common when the same type is fetched and then sent
// back to update -- is treated as a request. Over-requiring a response field is a runtime
// hazard, whereas over-requiring a request field is only a compile-time inconvenience.
func classifyMessages(gen *protogen.Plugin) {
	fromRequests := map[protoreflect.FullName]bool{}
	fromResponses := map[protoreflect.FullName]bool{}

	for _, f := range gen.Files {
		for _, service := range f.Services {
			for _, method := range service.Methods {
				walkMessage(method.Input, fromRequests)
				walkMessage(method.Output, fromResponses)
			}
		}
	}

	for name := range fromResponses {
		if !fromRequests[name] {
			responseOnlyMessages[name] = true
		}
	}
}

// walkMessage records a message and everything reachable through its fields.
func walkMessage(msg *protogen.Message, seen map[protoreflect.FullName]bool) {
	if msg == nil {
		return
	}
	name := msg.Desc.FullName()
	if seen[name] {
		return
	}
	seen[name] = true

	for _, field := range msg.Fields {
		if field.Desc.IsMap() {
			walkMessage(field.Message.Fields[1].Message, seen)
			continue
		}
		walkMessage(field.Message, seen)
	}
}

// isResponseOnlyMessage reports whether a message is never used in a request position.
func isResponseOnlyMessage(msg *protogen.Message) bool {
	return responseOnlyMessages[msg.Desc.FullName()]
}
