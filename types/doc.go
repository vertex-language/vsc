// Package types is the type model: what a program's types are, once
// the spellings in the source have been resolved.
//
// A type here is a value, and two of them are the same type when
// Identical says so. Nothing in this package reads source, holds a
// position, or reports a diagnostic — a type has no place in a file,
// and the same Int is every Int in the program.
//
// Three things it follows Swift on, because a Vertex value and a
// Swift value of the same type are the same bytes and obey the same
// rules:
//
// Layout. Size, stride and alignment are Swift's, extra inhabitants
// included, which is why `Int?` is nine bytes and `String?` is
// sixteen. See layout.go.
//
// Conformance is nominal. A type conforms to a protocol because it
// was declared to, never because its members happen to line up.
//
// A typealias is not a new type. It is another spelling of the one it
// names, and Identical looks through it.
package types
