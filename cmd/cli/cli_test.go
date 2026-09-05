package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/cmd/cli"
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
	if _, err := os.Stat(filepath.Join(dir, "hello")); err != nil {
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
		{"vil", "sil_stage lowered"},
		{"vir", `use "aarch64/macos"`},
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
// are Mach-O, and they are not an executable.
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
	if len(b) < 4 || b[0] != 0xcf || b[1] != 0xfa {
		t.Errorf("not a 64-bit Mach-O: % x", b[:min(8, len(b))])
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
	code, stdout, _ := run(t, "build", "--emit", "vil", "-o", "-", "-module", "lib", path)
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
	for _, want := range []string{"target\t", "entry\t_main", "sdk\t"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in:\n%s", want, stdout)
		}
	}
}
