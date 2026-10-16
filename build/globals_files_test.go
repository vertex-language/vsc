package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// TestModuleVarsInAnyOrderMatchSwift: a module-scope variable's
// initializer reads variables declared below it and in another file,
// which Swift allows because each is initialized on first use. The
// program is built with its files in both orders, and each prints what
// swiftc's does -- the trace of which initializer ran when included.
func TestModuleVarsInAnyOrderMatchSwift(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	files := map[string]string{
		"main.swift": `print("start")
print(n, total)
print(table)
`,
		// Reads what a.swift declares, and what it declares below.
		"b.swift": `let n = smallText.count
let total: Int = { print("total"); return words.count + n }()
let words = smallText.split(separator: " ")
let table: [String] = parse(tableText)
`,
		"a.swift": `let smallText: String = { print("smallText"); return "a b" }()
let tableText = "x y z"
func parse(_ s: String) -> [String] {
    return s.split(separator: " ").map { String($0) }
}
`,
	}
	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, order := range [][]string{
		{"main.swift", "b.swift", "a.swift"},
		{"main.swift", "a.swift", "b.swift"},
	} {
		t.Run(strings.Join(order, ","), func(t *testing.T) {
			var srcs []vsc.Source
			var paths []string
			for _, name := range order {
				path := filepath.Join(dir, name)
				srcs = append(srcs, vsc.Source{Name: path, Text: []byte(files[name])})
				paths = append(paths, path)
			}
			out := t.TempDir()
			oracle := filepath.Join(out, "oracle")
			if msg, err := exec.Command(swiftc, append([]string{"-o", oracle}, paths...)...).CombinedOutput(); err != nil {
				t.Fatalf("swiftc: %v\n%s", err, msg)
			}
			want, err := exec.Command(oracle).Output()
			if err != nil {
				t.Fatal(err)
			}

			unit, diags := vsc.Compile(srcs, vsc.Options{Module: "main", Target: target})
			if len(diags) > 0 {
				t.Fatalf("refused: %v", diags)
			}
			obj, err := build.Object(unit.VIR, build.Options{})
			if err != nil {
				t.Fatal(err)
			}
			exe, err := build.Executable([]build.Input{{Name: "main.o", Data: obj}}, build.LinkOptions{Target: target})
			if err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(out, "program")
			if err := os.WriteFile(bin, exe, 0o755); err != nil {
				t.Fatal(err)
			}
			got, err := exec.Command(bin).Output()
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("printed %q, swiftc's printed %q", got, want)
			}
		})
	}
}
