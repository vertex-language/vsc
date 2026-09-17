package build_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// TestReadLineMatchesSwift gives the same standard input to a program
// built by each compiler and compares what they print: lines ending in
// \n and \r\n and in nothing, bytes that are not UTF-8, and a String?
// taken apart every way the language has -- if let, guard let, while let,
// ??, ! and == nil. The arguments after the program's path go through
// CommandLine.arguments the same way.
func TestReadLineMatchesSwift(t *testing.T) {
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
	const program = `
func firstWord(_ line: String?) -> String? {
    guard let l = line else { return nil }
    return l.isEmpty ? nil : l
}

func main() -> Int32 {
    let header = readLine()
    print(header != nil, header == nil, header ?? "none")
    if let h = firstWord(header) { print("first:", h.debugDescription, h.count) }
    var n = 0
    while let line = readLine(strippingNewline: n % 2 == 0) {
        print(n, line.debugDescription, line.isEmpty)
        n += 1
    }
    let after = readLine()
    print(after == nil, after ?? "fallback", [n].isEmpty)
    let kept: String? = "kept"
    print(kept!, kept)
    let args = CommandLine.arguments
    var i = 1
    while i < args.count {
        print(i, args[i], args[i].count)
        i += 1
    }
    return 0
}
`
	input := []byte("héllo\r\nsecond\n\nbad \xff byte\r\nlast without newline")

	dir := t.TempDir()
	src := filepath.Join(dir, "main.swift")
	if err := os.WriteFile(src, []byte(program+"\n_ = main()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oracle := filepath.Join(dir, "oracle")
	if out, err := exec.Command(swiftc, "-module-name", "main", "-o", oracle, src).CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
	}
	want := runWithInput(t, oracle, input)

	unit, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(program)}},
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
	if got := runWithInput(t, bin, input); got != want {
		t.Errorf("output differs\n--- this compiler ---\n%s--- swiftc ---\n%s", got, want)
	}
}

func runWithInput(t *testing.T, path string, input []byte) string {
	t.Helper()
	cmd := exec.Command(path, "plain", "wörds with spaces", "")
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(out)
}
