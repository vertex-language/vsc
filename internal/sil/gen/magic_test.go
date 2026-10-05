package gen

import (
	"strings"
	"testing"
)

// TestMagicFileLiterals: #fileID names the module and the file's base name,
// and #file and #filePath the path the file was given by -- as swiftc
// spells them, whose own answer depends on the path it is handed.
func TestMagicFileLiterals(t *testing.T) {
	got, diags := generate(t, "lib", `
func id() -> String { return #fileID }
func path() -> String { return #filePath }
func file() -> String { return #file }
`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	for _, want := range []string{`"lib/main.swift"`, `"main.swift"`} {
		if !strings.Contains(got, want) {
			t.Errorf("no string %s in:\n%s", want, got)
		}
	}
}
