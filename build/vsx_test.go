package build_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vertex-language/ir"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// The .vsx ladder: tests/vsx/NNN-*.vsx, one program of ordinary .vsx
// per file (see its README). Markup has no oracle the way the Swift
// ladder has swiftc, so each file says what it prints, as the kernel
// ladder's do:
//
//	// want: <p>hi</p>      a line of standard output, in order
//
// Every .vsx program automatically receives the compiler's built-in .vsx v2
// prelude (analyzer/vsx_prelude.vs).

func vsxSources(t *testing.T, file string) []vsc.Source {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return []vsc.Source{{Name: filepath.Base(file), Text: src}}
}

// vsxFiles is the ladder, in order.
func vsxFiles(t *testing.T) []string {
	t.Helper()
	files, _ := filepath.Glob("../tests/vsx/*.vsx")
	if len(files) == 0 {
		t.Fatal("no programs in tests/vsx")
	}
	sort.Strings(files)
	return files
}

// vsxWant is what a file prints, from its `// want:` lines; ok is false
// where it has none.
func vsxWant(src []byte) (string, bool) {
	var lines []string
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "// want:":
			lines = append(lines, "")
		case strings.HasPrefix(line, "// want: "):
			lines = append(lines, strings.TrimPrefix(line, "// want: "))
		}
	}
	if len(lines) == 0 {
		return "", false
	}
	return strings.Join(lines, "\n") + "\n", true
}

// vsxWithWant is a program with its want lines replaced by out's.
func vsxWithWant(src []byte, out string) []byte {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(src), "\n"), "\n") {
		if t := strings.TrimSpace(line); t == "// want:" || strings.HasPrefix(t, "// want: ") {
			continue
		}
		b.WriteString(line + "\n")
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			b.WriteString("// want:\n")
		} else {
			b.WriteString("// want: " + line + "\n")
		}
	}
	return []byte(b.String())
}

func TestVSXRun(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	for _, file := range vsxFiles(t) {
		src := vsxSources(t, file)
		want, runs := vsxWant(src[0].Text)
		if !runs {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(file), ".vsx")
		t.Run(name, func(t *testing.T) {
			got := runVsxProgram(t, target, src)
			if got.crashed() || got.status != 0 {
				t.Errorf("the program ended with %s", got)
			}
			if os.Getenv("VSX_UPDATE") != "" {
				// VSX_UPDATE=1 writes what it printed as its want lines:
				// for a new program, whose output is then read and kept.
				if err := os.WriteFile(file, vsxWithWant(src[0].Text, got.stdout), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			if got.stdout != want {
				t.Errorf("output differs\n--- got ---\n%s\n--- want ---\n%s", clip(got.stdout), want)
			}
		})
	}
}

func runVsxProgram(t *testing.T, target ir.Target, srcs []vsc.Source) outcome {
	t.Helper()
	u, diags := vsc.Compile(srcs, vsc.Options{Module: "main", Target: target})
	if len(diags) > 0 {
		var b strings.Builder
		for _, d := range diags {
			b.WriteString("\n  ")
			b.WriteString(d.String())
		}
		t.Fatalf("vsc refused the program:%s", b.String())
	}
	obj, err := build.Object(u.VIR, build.Options{})
	if err != nil {
		t.Fatalf("vsc: %v", err)
	}
	exe, err := build.Executable([]build.Input{{Name: "main.o", Data: obj}}, build.LinkOptions{Target: target})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	bin := vsc.ImageName(target, filepath.Join(t.TempDir(), "main"))
	if err := os.WriteFile(bin, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	return run(t, bin)
}
