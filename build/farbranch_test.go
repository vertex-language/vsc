package build_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// TestFarBranchRuns: a function over a megabyte of machine code, whose
// first `if` skips all of it. On arm64 that conditional branch is past
// B.cond's ±1MB, and has to be written as the inverse over a B or the
// object does not assemble -- which a table of a few thousand struct
// literals, model/registry's once, ran into.
//
// The body is many `if`s of a few hundred statements each rather than one
// long run of them, because the ownership verifier is quadratic in the
// length of a block and the time would go there rather than to the backend.
func TestFarBranchRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a megabyte of code")
	}
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}

	const groups, stmts = 400, 300 // about 12 bytes a statement: 1.4MB
	var src strings.Builder
	src.WriteString("func work(_ x: Int) -> Int {\n    var t = x\n    if x >= 0 {\n")
	for g := 0; g < groups; g++ {
		fmt.Fprintf(&src, "        if t & %d == 0 {\n", 1<<(g%5))
		for s := 0; s < stmts; s++ {
			fmt.Fprintf(&src, "            t = (t &* 31) ^ %d\n", g*stmts+s)
		}
		fmt.Fprintf(&src, "        } else {\n            t = t &+ %d\n        }\n", g)
	}
	src.WriteString("    }\n    return t\n}\n\nfunc main() {\n    print(work(7), work(-1))\n}\n")

	work := func(x int64) int64 {
		t := x
		if x >= 0 {
			for g := 0; g < groups; g++ {
				if t&(1<<(g%5)) == 0 {
					for s := 0; s < stmts; s++ {
						t = t*31 ^ int64(g*stmts+s)
					}
				} else {
					t += int64(g)
				}
			}
		}
		return t
	}
	want := fmt.Sprintf("%d %d\n", work(7), work(-1))

	dir := t.TempDir()
	path := filepath.Join(dir, "main.vs")
	unit, diags := vsc.Compile([]vsc.Source{{Name: path, Text: []byte(src.String())}},
		vsc.Options{Module: "main", Target: target})
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
	if string(got) != want {
		t.Errorf("printed %q, want %q", got, want)
	}
}
