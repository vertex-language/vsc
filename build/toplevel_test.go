package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// TestTopLevelCodeMatchesSwift: main.swift's statements are the program,
// the way a SwiftPM executable target is written, with functions from
// another file of the module. swiftc builds the same two files, and the
// two programs have to print the same thing.
func TestTopLevelCodeMatchesSwift(t *testing.T) {
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
		"main.swift": `let greeting = "hello"
var total: Int32 = 0
var i: Int32 = 1
while i <= 4 {
    total += twice(i)
    i += 1
}
print(greeting, total)
if total > 10 {
    print("big", describe(total))
}
func describe(_ n: Int32) -> String {
    return "n=\(n)"
}
`,
		"math.swift": `func twice(_ x: Int32) -> Int32 {
    return x * 2
}
`,
	}
	dir := t.TempDir()
	var srcs []vsc.Source
	var paths []string
	for _, name := range []string{"main.swift", "math.swift"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil {
			t.Fatal(err)
		}
		srcs = append(srcs, vsc.Source{Name: path, Text: []byte(files[name])})
		paths = append(paths, path)
	}

	oracle := filepath.Join(dir, "oracle")
	if out, err := exec.Command(swiftc, append([]string{"-o", oracle}, paths...)...).CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
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
	bin := filepath.Join(dir, "program")
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
}
