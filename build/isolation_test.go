package build_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/iface"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// A module's interface says what is @MainActor, so that a client is held
// to the same rules the module was: a type marked so has its members
// written under it, with nonisolated on the ones that opt out; a
// function marked so is written so; and a client that calls into either
// from a synchronous nonisolated context is refused, in Swift's words.
func TestInterfaceCarriesMainActor(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	const libSrc = `
@MainActor
public final class Screen {
    public var frames: Int32 = 0
    public init() {}
    public func draw() { frames += 1 }
    nonisolated public func name() -> Int32 { return 7 }
}

@MainActor public func redraw() {}

public func compute() -> Int32 { return 1 }
`
	lib, diags := vsc.Compile([]vsc.Source{{Name: "lib.swift", Text: []byte(libSrc)}},
		vsc.Options{Module: "Lib", Target: target})
	for _, d := range diags {
		t.Fatalf("lib: %v", d)
	}
	var ifb bytes.Buffer
	if err := iface.Print(&ifb, iface.Module{
		Name: "Lib", Files: lib.Files, Units: lib.Positions, Info: lib.Info,
	}); err != nil {
		t.Fatal(err)
	}
	text := ifb.String()
	for _, want := range []string{
		"@MainActor public class Screen",
		"  nonisolated public func name() -> int32",
		"  public func draw()",
		"  public var frames: int32",
		"@MainActor public func redraw()",
		"\npublic func compute() -> int32",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("interface lacks %q:\n%s", want, text)
		}
	}

	ifFile := token.NewFile("Lib.vinterface", ifb.Bytes())
	ifAST, ds := parser.ParseFile(ifFile, 0)
	for _, d := range ds {
		t.Fatalf("interface does not parse: %s", d.Print(ifFile))
	}
	const appSrc = `
func worker(_ s: Screen) -> Int32 {
    s.draw()
    redraw()
    return s.name() + s.frames + compute()
}
`
	appFile := token.NewFile("app.swift", []byte(appSrc))
	appAST, ds := parser.ParseFile(appFile, 0)
	for _, d := range ds {
		t.Fatalf("app parse: %s", d.Print(appFile))
	}
	_, checks := analyzer.CheckImporting([]*ast.File{appAST}, []analyzer.Import{{
		Name: "Lib", Files: []*ast.File{ifAST}, Units: []*token.File{ifFile},
	}})
	var got []string
	for _, d := range checks {
		got = append(got, d.Message)
	}
	want := []string{
		"call to main actor-isolated instance method 'draw()' in a synchronous nonisolated context",
		"call to main actor-isolated global function 'redraw()' in a synchronous nonisolated context",
		"main actor-isolated property 'frames' can not be referenced from a nonisolated context",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("client diagnostics:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
