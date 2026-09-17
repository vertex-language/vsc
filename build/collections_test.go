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

// TestCollectionsMatchSwift builds one program with each compiler and
// compares what they print: Array mutation and copy on write, Dictionary
// and Set lookups, insertion and removal, and their descriptions. A
// Dictionary or Set of more than one entry is never printed, because the
// order of its entries differs between runs in both languages.
func TestCollectionsMatchSwift(t *testing.T) {
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
	src, err := os.ReadFile(filepath.Join("..", "tests", "collections", "main.swift"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	oracleSrc := filepath.Join(dir, "main.swift")
	if err := os.WriteFile(oracleSrc, append(src, []byte("\n_ = main()\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	oracle := filepath.Join(dir, "oracle")
	if out, err := exec.Command(swiftc, "-module-name", "main", "-o", oracle, oracleSrc).CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
	}
	want, err := exec.Command(oracle).Output()
	if err != nil {
		t.Fatal(err)
	}

	unit, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: src}},
		vsc.Options{Module: "main", Target: target})
	if len(diags) > 0 {
		var b strings.Builder
		for _, d := range diags {
			b.WriteString("\n  " + d.String())
		}
		t.Fatalf("this compiler refused the program:%s", b.String())
	}
	obj, err := build.Object(unit.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	exe, err := build.Executable([]build.Input{{Name: "main.o", Data: obj}}, build.LinkOptions{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "vsc")
	if err := os.WriteFile(bin, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("running this compiler's program: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("output differs\n--- this compiler ---\n%s--- swiftc ---\n%s", got, want)
	}
}
