package pkg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveMixedPackage lays out build/testdata/packages/001-mixed: SwiftPM's
// directories, each target's files and language, the public headers, the
// settings, and an order that builds every target after what it needs.
func TestResolveMixedPackage(t *testing.T) {
	dir := "../build/testdata/packages/001-mixed"
	m, diags, err := Load(dir)
	if err != nil || len(diags) > 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	p, err := Resolve(dir, m, "macos", "debug")
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, rt := range p.Targets {
		order = append(order, rt.Name)
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["CMath"] > pos["Geometry"] || pos["Geometry"] > pos["App"] || pos["CxxStats"] > pos["App"] {
		t.Errorf("build order %v puts a target before a dependency", order)
	}

	cmath := p.Target("CMath")
	if cmath.Swift() || len(cmath.Sources) != 1 || cmath.Sources[0].Language != CXX ||
		filepath.Base(cmath.PublicHeaders) != "include" || strings.Join(cmath.Defines["c"], " ") != "SCALE=3" {
		t.Errorf("CMath = %+v", cmath)
	}
	stats := p.Target("CxxStats")
	if !stats.UsesCXX() || len(stats.HeaderSearchPaths) != 1 || filepath.Base(stats.HeaderSearchPaths[0]) != "detail" {
		t.Errorf("CxxStats = %+v", stats)
	}
	app := p.Target("App")
	if !app.Swift() || len(app.Dependencies) != 2 || strings.Join(app.Libraries, " ") != "c++" {
		t.Errorf("App = %+v", app)
	}
	var closure []string
	for _, rt := range app.Closure() {
		closure = append(closure, rt.Name)
	}
	if len(closure) != 4 || closure[len(closure)-1] != "App" {
		t.Errorf("App's closure = %v", closure)
	}
	exes := p.Executables()
	if len(exes) != 1 || exes[0].Name != "mixed" || exes[0].Target != app {
		t.Errorf("executables = %+v", exes)
	}
}

// TestResolveRefusesACycle: SwiftPM's words for a target that needs itself.
func TestResolveRefusesACycle(t *testing.T) {
	m := &Manifest{Name: "P", Targets: []Target{
		{Name: "A", Kind: TargetRegular, Dependencies: []TargetDependency{{Kind: "byName", Name: "B"}}},
		{Name: "B", Kind: TargetRegular, Dependencies: []TargetDependency{{Kind: "byName", Name: "A"}}},
	}}
	root := t.TempDir()
	for _, n := range []string{"A", "B"} {
		mustWrite(t, filepath.Join(root, "Sources", n, n+".swift"), "public func f() {}\n")
	}
	_, err := Resolve(root, m, "macos", "debug")
	if err == nil || !strings.Contains(err.Error(), "cyclic dependency declaration found: A -> B -> A") {
		t.Errorf("err = %v", err)
	}
}

func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
