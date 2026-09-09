package pass

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/text"
)

// TestAgreesWithSwiftc is this package's oracle.
//
// vil/gen is held to `swiftc -emit-silgen`, the raw form. This package
// makes the canonical form, so it is held to `swiftc -emit-sil`, which
// is what swiftc prints after its own mandatory passes have run.
//
// The corpus is arithmetic and control flow on purpose. A function
// that owns something diverges here for a reason already written down:
// `-emit-sil` has had the ARC optimizer over it, and where we emit a
// retain immediately followed by a release, swiftc prints neither.
// These programs retain nothing, so there is nothing for that
// optimizer to have done, and the two outputs are the same program.
//
// Four kinds of program are deliberately not in the corpus, each for a
// reason worth knowing:
//
//   - `/` and `%`. Swift's division checks for a zero divisor first,
//     and the check is written in the standard library: swiftc inlines
//     a call to _precondition with a StaticString message and a source
//     location. There is no library here to inline, so we emit the
//     bare sdiv. That is a real difference in behaviour and not only
//     in text -- our division by zero does whatever the hardware does.
//
//   - `<<` and `>>`. Swift's shift operators are the smart ones: a
//     negative or over-wide amount is defined, not undefined, and the
//     definition is a stdlib function full of branches. We emit the
//     masking shift, which is what `&<<` means.
//
//   - `&+`, `&-`, `&*`. The wrapping operators are not declared in
//     core yet: they are the reporting builtin with the reporting
//     turned off, and core says how to lower an operator but not yet
//     that one.
//
//   - Anything with an `if` in it. swiftc joins the arms at a block
//     that takes the result as an argument; we return from each arm.
//     Both are correct and the second is not yet the first. The same
//     goes for a literal, where swiftc has already folded
//     struct_extract of struct back to the value inside and we have
//     not.
func TestAgreesWithSwiftc(t *testing.T) {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	for _, path := range corpus(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, name := program(t, path)
			m := canonical(t, path)
			ours := text.Normalize(funcText(t, m, name))
			theirs := text.Normalize(swiftSIL(t, swiftc, src, name))
			if ours != theirs {
				t.Errorf("VIL and SIL disagree.\n--- vil\n%s\n--- sil\n%s", ours, theirs)
			}
		})
	}
}

// TestCorpusIsCanonical holds the same programs to the rules, with or
// without a toolchain to compare against. Agreeing with swiftc and
// being sound are different claims.
func TestCorpusIsCanonical(t *testing.T) {
	for _, path := range corpus(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			canonical(t, path)
		})
	}
}

// canonical lowers one corpus program and runs the passes over it.
func canonical(t *testing.T, path string) *vil.Module {
	t.Helper()
	m := lowerFile(t, path)
	if err := Mandatory(m); err != nil {
		t.Fatalf("mandatory: %v", err)
	}
	if err := LowerOwnership(m); err != nil {
		t.Fatal(err)
	}
	return m
}

func corpus(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("testdata/*.swift")
	if err != nil || len(files) == 0 {
		t.Skip("no corpus")
	}
	return files
}

func program(t *testing.T, path string) (src, fn string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src = string(b)
	first, _, _ := strings.Cut(src, "\n")
	name, ok := strings.CutPrefix(strings.TrimSpace(first), "// vil:")
	if !ok {
		t.Fatalf("%s: the first line must name the function to compare, as `// vil: name`", path)
	}
	return src, strings.TrimSpace(name)
}

// swiftSIL runs swiftc over the same source and returns one function's
// canonical SIL.
func swiftSIL(t *testing.T, swiftc, src, fn string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "t.swift")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(swiftc, "-emit-sil", path).Output()
	if err != nil {
		t.Skipf("swiftc: %v", err)
	}

	var keep []string
	in := false
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "sil ") && strings.Contains(line, fn) {
			in = true
		}
		if in {
			keep = append(keep, line)
			if strings.HasPrefix(line, "} // end sil function") {
				break
			}
		}
	}
	if len(keep) == 0 {
		t.Skipf("swiftc emitted no function matching %q", fn)
	}
	return strings.Join(keep, "\n") + "\n"
}

func funcText(t *testing.T, m *vil.Module, name string) string {
	t.Helper()
	f := m.LookupSource(name)
	if f == nil {
		t.Fatalf("no function %q in the module:\n%s", name, text.String(m))
	}
	var b strings.Builder
	if err := text.Func(&b, f); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
