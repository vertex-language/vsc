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

// TestGenericsAcrossFilesMatchSwift: a generic function written in one
// file of the module and called from another is specialized from the file
// it was written in. Its body's positions mean nothing in the caller's
// file, which is longer or shorter than the one they came from. swiftc
// builds the same two files, and the two programs print the same thing.
func TestGenericsAcrossFilesMatchSwift(t *testing.T) {
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
		"main.swift": `let a = firstOf([7, 8], or: 1)
let b = firstOf([String](), or: "fallback")
print(a, b, pair(2, 3).count)
`,
		"helpers.swift": `// Padding, so that positions in this file run past the end of main.swift,
// and an offset read in the wrong file lands nowhere or somewhere else.
// More of it, to be sure.
func firstOf<T>(_ xs: [T], or fallback: T) -> T {
    if xs.isEmpty {
        return fallback
    }
    return xs[0]
}

func pair<T>(_ x: T, _ y: T) -> [T] {
    return [x, y]
}
`,
	}
	dir := t.TempDir()
	var srcs []vsc.Source
	var paths []string
	for _, name := range []string{"main.swift", "helpers.swift"} {
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
