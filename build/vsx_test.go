package build_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/vertex-language/ir"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/token"
)

// The .vsx ladder: tests/vsx (see its README). Markup has no oracle the
// way the Swift ladder has swiftc, so a run test writes down its output
// in a .out file beside it, and an error test marks each line that has
// to be refused with `// want "part of the message"`.
//
// Every program is compiled with tests/vsx/prelude.vs, the stand-in for
// ui/component and web/dom that markup's check form calls.

func vsxSources(t *testing.T, file string) []vsc.Source {
	t.Helper()
	prelude, err := os.ReadFile("../tests/vsx/prelude.vs")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return []vsc.Source{{Name: "prelude.vs", Text: prelude}, {Name: filepath.Base(file), Text: src}}
}

func TestVSXRun(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	files, _ := filepath.Glob("../tests/vsx/run/*.vsx")
	if len(files) == 0 {
		t.Fatal("no programs in tests/vsx/run")
	}
	sort.Strings(files)
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".vsx")
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile(strings.TrimSuffix(file, ".vsx") + ".out")
			if err != nil {
				t.Fatalf("no %s.out beside it", name)
			}
			got := runVsxProgram(t, target, vsxSources(t, file))
			if got.crashed() || got.status != 0 {
				t.Errorf("the program ended with %s", got)
			}
			if got.stdout != string(want) {
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

var wantRE = regexp.MustCompile(`// want "([^"]*)"`)

func TestVSXErrors(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	files, _ := filepath.Glob("../tests/vsx/errors/*.vsx")
	if len(files) == 0 {
		t.Fatal("no programs in tests/vsx/errors")
	}
	sort.Strings(files)
	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".vsx"), func(t *testing.T) {
			srcs := vsxSources(t, file)
			// What each line has to be refused with.
			want := map[int][]string{}
			for i, line := range strings.Split(string(srcs[1].Text), "\n") {
				for _, m := range wantRE.FindAllStringSubmatch(line, -1) {
					want[i+1] = append(want[i+1], m[1])
				}
			}
			if len(want) == 0 {
				t.Fatal(`no // want "…" in the file`)
			}
			_, diags := vsc.Compile(srcs, vsc.Options{Module: "main", Target: target})
			got := map[int][]string{}
			for _, d := range diags {
				if d.Severity != token.Error {
					continue
				}
				if d.File == nil || !strings.HasSuffix(d.File.Name(), filepath.Base(file)) {
					t.Errorf("unexpected error outside the program: %s", d)
					continue
				}
				line := d.File.Position(d.Pos).Line
				got[line] = append(got[line], d.Message)
			}
			for line, parts := range want {
				for _, part := range parts {
					found := false
					for _, msg := range got[line] {
						if strings.Contains(msg, part) {
							found = true
						}
					}
					if !found {
						t.Errorf("line %d: want an error containing %q, got %q", line, part, got[line])
					}
				}
			}
			for line, msgs := range got {
				if len(want[line]) == 0 {
					t.Errorf("line %d: unexpected error(s) %q", line, msgs)
				}
			}
		})
	}
}
