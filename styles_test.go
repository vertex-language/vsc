package vsc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A program's folder with .vss files gets its stylesheet as a generated
// source, once, and a folder with none gets nothing.
func TestProgramStyles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	main := write("main.vs", "package main\n\nfunc main() -> int32 { return 0 }\n")
	write("a.vss", "package main\n.x { color: red }\n")
	write("b.vss", "package main\n:root { --y: 1 }\n")
	u, diags := Compile([]Source{{Name: main, Text: []byte("package main\n\nfunc main() -> int32 { return 0 }\n")}}, Options{Module: "main", Stop: Parsed})
	for _, d := range diags {
		t.Errorf("%s", d)
	}
	var gen string
	for _, f := range u.Positions {
		if strings.HasPrefix(filepath.Base(f.Name()), "__vss_") {
			gen = string(f.Text())
		}
	}
	if gen == "" {
		t.Fatal("no generated stylesheet source")
	}
	for _, want := range []string{"package main", "public let __vssSheet", `@layer main {`, `:root { --y: 1 }`, `.x { color: red }`} {
		if !strings.Contains(gen, want) {
			t.Errorf("generated source lacks %q:\n%s", want, gen)
		}
	}

	write("c.vss", "package other\n")
	_, diags = Compile([]Source{{Name: main, Text: []byte("package main\n")}}, Options{Module: "main", Stop: Parsed})
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "this .vss file is package other") {
			found = true
		}
	}
	if !found {
		t.Errorf("a .vss file of another package is not reported: %v", diags)
	}
}
