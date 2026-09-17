package build_test

import (
	"bytes"
	"strings"
	"testing"

	irtext "github.com/vertex-language/ir/text"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// TestWindowsReturnsWideResultsThroughStorage: under the Microsoft
// convention one register comes back, so a String -- two words -- comes
// back through a hidden pointer, both from a function this compiler
// defines and from the runtime's, whose C++ vcx lowered the same way.
// Every such call on every machine is checked here as IR and as an
// object, since running the program needs Windows.
func TestWindowsReturnsWideResultsThroughStorage(t *testing.T) {
	target, ok := vsc.LookupTarget("x86_64-windows")
	if !ok {
		t.Skip("no x86_64-windows target")
	}
	const program = `
func greeting(_ name: String) -> String { return "hello, " + name }

func main() -> Int32 {
    let s = greeting("a name long enough to be stored")
    print(s, s.count, 1.5, [1, 2, 3].count)
    return 0
}
`
	unit, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(program)}},
		vsc.Options{Module: "main", Target: target})
	if len(diags) > 0 {
		t.Fatalf("refused: %v", diags)
	}
	var text bytes.Buffer
	if err := irtext.Print(&text, unit.VIR); err != nil {
		t.Fatal(err)
	}
	ir := text.String()
	for _, want := range []string{"@vertex_string_concat", "@vertex_string_literal", "$s4main8greetingyS2SF"} {
		found := false
		for _, line := range strings.Split(ir, "\n") {
			if strings.Contains(line, "func") && strings.Contains(line, want) {
				found = true
				if !strings.Contains(line, "sret") {
					t.Errorf("%s does not return through storage:\n%s", want, line)
				}
			}
		}
		if !found {
			t.Errorf("no declaration of %s in:\n%s", want, ir)
		}
	}
	if _, err := build.Object(unit.VIR, build.Options{}); err != nil {
		t.Fatalf("object: %v", err)
	}
	if _, err := build.Runtime(target); err != nil {
		t.Fatalf("runtime: %v", err)
	}
}
