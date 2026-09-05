package analyzer_test

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// Initializers, and the arguments a call to one has to supply.
//
// A struct that declares none gets the memberwise initializer: one
// parameter per stored property, in declaration order, labelled with
// the property's name, and defaulted where the property has an
// initial value. Before this was modelled the checker said nothing
// about a constructor's arguments at all — its own comment said so —
// and a call with the wrong labels, the wrong count or the wrong
// types went through to the generator unexamined.

func check(t *testing.T, src string) []string {
	t.Helper()
	f := token.NewFile("t.swift", []byte(src))
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	_, checks := analyzer.Check([]*ast.File{file})
	msgs := make([]string, 0, len(checks))
	for _, d := range checks {
		msgs = append(msgs, d.Message)
	}
	return msgs
}

func TestMemberwiseInitializerIsChecked(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"too few arguments", `
struct P { var x: Int; var y: Int }
func use() { _ = P(x: 1) }
`, "argument count"},

		{"labels out of order", `
struct P { var x: Int; var y: Int }
func use() { _ = P(y: 1, x: 2) }
`, "argument label"},

		{"the wrong type", `
struct P { var x: Int }
func use() { _ = P(x: "no") }
`, "cannot convert"},
	} {
		t.Run(c.name, func(t *testing.T) {
			msgs := check(t, c.src)
			if len(msgs) == 0 {
				t.Fatalf("accepted %q", c.src)
			}
			if !strings.Contains(strings.Join(msgs, "\n"), c.want) {
				t.Errorf("said %v, want something about %q", msgs, c.want)
			}
		})
	}
}

// TestMemberwiseAcceptsWhatSwiftAccepts: the point of modelling the
// initializer is to reject what Swift rejects, and nothing else.
func TestMemberwiseAcceptsWhatSwiftAccepts(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"every property given", `
struct P { var x: Int; var y: Int }
func use() { _ = P(x: 1, y: 2) }
`},
		// A property with an initial value gives its parameter a
		// default, so a struct whose properties all have one is
		// makeable with no arguments at all.
		{"all properties defaulted", `
struct P { var x = 0; var y = 0 }
func use() { _ = P() }
`},
		{"some properties defaulted", `
struct P { var x: Int; var y = 0 }
func use() { _ = P(x: 1) }
`},
		{"generic, inferred from the argument", `
struct Wrapper<T> { var value: T }
func use() { _ = Wrapper(value: 3) }
`},
		// A type that says how it is made does not also get the free
		// answer, so nothing here is checked against a memberwise
		// signature that should not exist.
		{"an initializer of its own", `
struct P {
    var x: Int
    init(both: Int) { x = both }
}
func use() { _ = P(both: 1) }
`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if msgs := check(t, c.src); len(msgs) != 0 {
				t.Errorf("rejected a program Swift accepts: %v", msgs)
			}
		})
	}
}

// TestDefaultArgumentsMayBeOmitted was a gap before the memberwise
// initializer needed it: a defaulted parameter had to be passed
// anyway, for an ordinary function as much as for an initializer.
func TestDefaultArgumentsMayBeOmitted(t *testing.T) {
	for _, src := range []string{
		`func f(_ a: Int = 1) -> Int { return a }
func use() { _ = f() }`,
		`func f(a: Int = 1, b: Int = 2) -> Int { return a }
func use() { _ = f(b: 3) }`,
		`func f(a: Int, b: Int = 2) -> Int { return a }
func use() { _ = f(a: 1) }`,
	} {
		if msgs := check(t, src); len(msgs) != 0 {
			t.Errorf("rejected %q: %v", src, msgs)
		}
	}
}

// TestAMissingArgumentIsNamed: with defaults in play, counting is not
// enough — which parameter went unsupplied is the useful half.
func TestAMissingArgumentIsNamed(t *testing.T) {
	msgs := check(t, `
func f(a: Int, b: Int = 2) -> Int { return a }
func use() { _ = f(b: 3) }
`)
	if len(msgs) == 0 {
		t.Fatal("a call missing a required argument was accepted")
	}
	if !strings.Contains(strings.Join(msgs, "\n"), "'a'") {
		t.Errorf("said %v, want it to name the missing parameter", msgs)
	}
}
