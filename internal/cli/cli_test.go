package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/internal/cli"
)

const program = `
func fib(_ n: Int32) -> Int32 {
    if n < 2 { return n }
    return fib(n - 1) + fib(n - 2)
}

func main() -> Int32 { return fib(10) }
`

// run drives the command the way a shell does and returns everything
// it said.
func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = cli.Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

// write puts a source file in a temporary directory and returns its
// path.
func write(t *testing.T, name, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// hosted skips a test that needs this machine to be a target.
func hosted(t *testing.T) {
	t.Helper()
	if vsc.HostName() == "" {
		t.Skip("this host is not a target vsc models")
	}
}

// TestUsage: a command with nothing to do says what it is for, and
// says it as an error rather than as output.
func TestUsage(t *testing.T) {
	code, stdout, stderr := run(t)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("usage went to stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "vsc build") {
		t.Errorf("usage does not list the verbs:\n%s", stderr)
	}
}

// TestHelpIsOutput: asked for, the same text is output and a success.
// `vsc help | less` should work and `vsc` alone should not pollute a
// pipe.
func TestHelpIsOutput(t *testing.T) {
	code, stdout, _ := run(t, "help")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "vsc build") {
		t.Errorf("help does not list the verbs:\n%s", stdout)
	}
}

func TestUnknownVerb(t *testing.T) {
	code, _, stderr := run(t, "frobnicate")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "frobnicate") {
		t.Errorf("the message does not name the verb: %q", stderr)
	}
}

// TestCheck: the exit code is the contract, since `vsc check x && ...`
// is what a script writes.
func TestCheck(t *testing.T) {
	hosted(t)
	path := write(t, "ok.vs", program)
	if code, _, stderr := run(t, "check", path); code != 0 {
		t.Errorf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
}

// TestCheckReportsErrors: a type error is exit 1, sited, and drawn
// with the line and a caret under it.
func TestCheckReportsErrors(t *testing.T) {
	hosted(t)
	path := write(t, "bad.vs", "func main() -> Int32 {\n    let x: Int32 = \"hello\"\n    return x\n}\n")
	code, _, stderr := run(t, "check", path)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	for _, want := range []string{"bad.vs:2:", "error:", "let x: Int32 = ", "^"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("missing %q in:\n%s", want, stderr)
		}
	}
}

// TestBuildAndRunTheProgram: the artifact is executable without a
// chmod, and it computes what the source says.
func TestBuildAndRunTheProgram(t *testing.T) {
	hosted(t)
	src := write(t, "fib.vs", program)
	out := filepath.Join(filepath.Dir(src), "fib")
	if code, _, stderr := run(t, "build", "-o", out, src); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	cmd := exec.Command(out)
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run: %v", err)
		}
	}
	if got := cmd.ProcessState.ExitCode(); got != 55 {
		t.Errorf("exit status = %d, want 55", got)
	}
}

// TestBuildNamesTheOutput: with no -o, the artifact is the input's
// base name, which is what every compiler does.
func TestBuildNamesTheOutput(t *testing.T) {
	hosted(t)
	src := write(t, "hello.vs", "func main() {}")
	dir := filepath.Dir(src)

	// The name is relative to the working directory, so the test has
	// to be in the one the artifact should land in. t.Chdir would say
	// this in one line and needs Go 1.24; this module is 1.23.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	if code, _, stderr := run(t, "build", "hello.vs"); code != 0 {
		t.Fatalf("exit = %d; stderr:\n%s", code, stderr)
	}
	target, _ := vsc.HostTarget()
	name := vsc.ImageName(target, "hello")
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Errorf("no artifact named for the input: %v", err)
	}
}

// TestRunForwardsTheExitCode: a runner that swallows it cannot be
// used in a script.
func TestRunForwardsTheExitCode(t *testing.T) {
	hosted(t)
	path := write(t, "fib.vs", program)
	if code, _, stderr := run(t, "run", path); code != 55 {
		t.Errorf("exit = %d, want 55; stderr:\n%s", code, stderr)
	}
}

// TestEmit: each mode produces its own artifact, and -o - puts it on
// standard output.
func TestEmit(t *testing.T) {
	hosted(t)
	path := write(t, "fib.vs", program)
	for _, c := range []struct{ mode, want string }{
		{"sil", "sil_stage lowered"},
		{"vir", `use "` + hostUse() + `"`},
	} {
		t.Run(c.mode, func(t *testing.T) {
			code, stdout, stderr := run(t, "build", "--emit", c.mode, "-o", "-", path)
			if code != 0 {
				t.Fatalf("exit = %d; stderr:\n%s", code, stderr)
			}
			if !strings.Contains(stdout, c.want) {
				t.Errorf("missing %q in:\n%s", c.want, stdout)
			}
		})
	}
}

// TestEmitObject writes an object rather than a program: the bytes
// are the target's container, and they are not an executable.
func TestEmitObject(t *testing.T) {
	hosted(t)
	src := write(t, "fib.vs", program)
	out := filepath.Join(filepath.Dir(src), "fib.o")
	if code, _, stderr := run(t, "build", "--emit", "obj", "-o", out, src); code != 0 {
		t.Fatalf("exit = %d; stderr:\n%s", code, stderr)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// The first two bytes say which container. Mach-O opens with
	// its 64-bit magic, little-endian; a COFF object opens with the
	// machine, and 0x8664 is AMD64.
	var magic [2]byte
	var what string
	switch vsc.HostName() {
	case "aarch64-macos":
		magic, what = [2]byte{0xcf, 0xfa}, "a 64-bit Mach-O"
	case "x86_64-windows":
		magic, what = [2]byte{0x64, 0x86}, "an AMD64 COFF object"
	default:
		t.Skipf("no container magic written down for %s", vsc.HostName())
	}
	if len(b) < 2 || b[0] != magic[0] || b[1] != magic[1] {
		t.Errorf("not %s: % x", what, b[:min(8, len(b))])
	}
}

func TestUnknownEmit(t *testing.T) {
	hosted(t)
	path := write(t, "fib.vs", program)
	code, _, stderr := run(t, "build", "--emit", "wasm", path)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "wasm") || !strings.Contains(stderr, "exe") {
		t.Errorf("the message names neither the mistake nor the choices: %q", stderr)
	}
}

// TestUnknownTarget: the message names the flag that fixes it, which
// is the one thing the library cannot say.
func TestUnknownTarget(t *testing.T) {
	path := write(t, "fib.vs", program)
	code, _, stderr := run(t, "check", "-target", "vax-ultrix", path)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "vax-ultrix") || !strings.Contains(stderr, "aarch64-macos") {
		t.Errorf("the message does not name the target or the choices: %q", stderr)
	}
}

// TestRunRefusesACrossBuild: run starts what it built, so a target
// that is not this machine is caught before the build rather than
// after it.
func TestRunRefusesACrossBuild(t *testing.T) {
	hosted(t)
	path := write(t, "fib.vs", program)
	code, _, stderr := run(t, "run", "-target", "vax-ultrix", path)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "this machine") {
		t.Errorf("the message does not say why: %q", stderr)
	}
}

// TestModuleDecidesTheEntryPoint: a library's main is an ordinary
// function, so a library has no entry point to link and building one
// as a program is refused rather than silently mislinked.
func TestModuleDecidesTheEntryPoint(t *testing.T) {
	hosted(t)
	path := write(t, "lib.vs", "public func main() {}")
	code, stdout, _ := run(t, "build", "--emit", "sil", "-o", "-", "-module", "lib", path)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(stdout, "@main :") {
		t.Errorf("a library got an entry point:\n%s", stdout)
	}
}

// TestTokensAndAST: the two inspection verbs, which go to the scanner
// and the parser and never further.
func TestTokensAndAST(t *testing.T) {
	path := write(t, "fib.vs", program)
	if code, stdout, _ := run(t, "tokens", path); code != 0 || !strings.Contains(stdout, `func	"func"`) {
		t.Errorf("tokens: exit %d, output:\n%s", code, stdout)
	}
	if code, stdout, _ := run(t, "ast", path); code != 0 || !strings.Contains(stdout, "FuncDecl") {
		t.Errorf("ast: exit %d, output:\n%s", code, stdout)
	}
}

// TestASTDumpsABrokenParse: a tree is worth looking at exactly when
// the parse went wrong, so a diagnostic does not take the tool away.
func TestASTDumpsABrokenParse(t *testing.T) {
	path := write(t, "broken.vs", "func f(] {}\n")
	code, stdout, stderr := run(t, "ast", path)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, "FuncDecl") {
		t.Errorf("no tree for a broken parse:\n%s", stdout)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("no diagnostic for a broken parse:\n%s", stderr)
	}
}

// TestEnv prints what a build depends on and cannot see.
func TestEnv(t *testing.T) {
	hosted(t)
	code, stdout, _ := run(t, "env")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"target\t", "entry\t" + hostEntry(), "libdirs\t"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in:\n%s", want, stdout)
		}
	}
}

// hostUse and hostEntry are what this machine's target answers, for
// the tests that check a message naming one. Asked rather than
// spelled out: a test that writes one target's answer down is a test
// that only runs on that target.
func hostUse() string {
	t, _ := vsc.HostTarget()
	return t.Use()
}

func hostEntry() string {
	t, _ := vsc.HostTarget()
	return vsc.EntrySymbol(t)
}

// TestAnImportedPackageBuildsItsCxxModule: a folder imported by a program
// brings its C++ module, whose exports its Vertex calls unqualified -- the
// module is more of the package -- and whose objects the program links.
// The vs.mod asks for a later macOS than the default, and the program is
// linked for that one.
func TestAnImportedPackageBuildsItsCxxModule(t *testing.T) {
	hosted(t)
	if !strings.HasSuffix(vsc.HostName(), "macos") {
		t.Skip("the module declares a macOS deployment target")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"vs.mod": "module github.com/you/answers\n\nplatform macos 14\n",
		"lib/answers/answers.cpp": `module;
#include <cstdint>
#include <string_view>
export module answers;

export enum class Kind : int32_t { small = 1, large = 40 };
export int32_t value(Kind k) noexcept { return static_cast<int32_t>(k); }
export int32_t length(std::string_view s) noexcept { return static_cast<int32_t>(s.size()); }
`,
		"lib/answers/answers.vs": `package answers

public func Answer() -> int32 {
    return value(Kind.large) + length("xy")
}
`,
		"main.vs": `package main

import "./lib/answers"

func main() -> int32 { return answers.Answer() }
`,
	})
	out := filepath.Join(root, "answer")
	if code, _, stderr := run(t, "build", "-o", out, filepath.Join(root, "main.vs")); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	cmd := exec.Command(out)
	_ = cmd.Run()
	if got := cmd.ProcessState.ExitCode(); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestAProgramByNameIsItsCmdFolder: `vsc build check` in a checkout is its
// cmd/check, whose imports of the checkout's own packages -- the root, a C++
// module, and a folder of it -- are read from disk, and nothing is fetched.
// A folder imported by path declares the module its path is.
func TestAProgramByNameIsItsCmdFolder(t *testing.T) {
	hosted(t)
	root := filepath.Join(t.TempDir(), "answers")
	writeTree(t, root, map[string]string{
		"vs.mod": "module github.com/vertex-language/answers\n",
		"answers.cpp": `export module answers;
export int answer() { return 40; }
`,
		"extra/extra.vs": `package extra

public func Two() -> int32 { return 2 }
`,
		"cmd/check/main.vs": `package main

import "answers"
import "answers/extra"

func main() -> int32 { return answers.answer() + extra.Two() }
`,
	})
	t.Chdir(root)
	out := filepath.Join(t.TempDir(), "check")
	code, _, stderr := run(t, "build", "-offline", "-o", out, "check")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	cmd := exec.Command(out)
	_ = cmd.Run()
	if got := cmd.ProcessState.ExitCode(); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}

	// The module a folder declares is its path's.
	writeTree(t, root, map[string]string{"answers.cpp": "export module wrong;\nexport int answer() { return 40; }\n"})
	code, _, stderr = run(t, "build", "-offline", "-o", out, "check")
	if code == 0 || !strings.Contains(stderr, "export module answers;") {
		t.Errorf("a misnamed module built (exit %d):\n%s", code, stderr)
	}
}

// TestCxxImportsAnotherPackagesModule: a package's C++ imports another
// package's module, and the runtime's task module, by name. The program
// imports only the first package, and still links the second's objects.
func TestCxxImportsAnotherPackagesModule(t *testing.T) {
	hosted(t)
	root := filepath.Join(t.TempDir(), "p")
	writeTree(t, root, map[string]string{
		"vs.mod": "module github.com/me/p\n",
		"math/math.cpp": `export module p.math;
export int add(int a, int b) { return a + b; }
`,
		"calc/calc.cpp": `export module p.calc;
import p.math;
import vertex.task;
export int twice(int x) { return add(x, x) + (vertex_task_workers() >= 0 ? 0 : 100); }
`,
		"cmd/t/main.vs": `package main

import "github.com/me/p/calc"

func main() -> int32 { return calc.twice(21) }
`,
	})
	t.Chdir(root)
	out := filepath.Join(t.TempDir(), "t")
	if code, _, stderr := run(t, "build", "-offline", "-o", out, "t"); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	cmd := exec.Command(out)
	_ = cmd.Run()
	if got := cmd.ProcessState.ExitCode(); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
