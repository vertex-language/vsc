package gen

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil/text"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// TestImplicitSelfInMutatingMethod: inside a mutating method self is an
// address, and a method that does not mutate, called without writing
// self, takes self's value -- a copy loaded out of the address, as
// `self.nonce()` would pass. Passing the address itself went unnoticed
// until the callee was in another module, where lowering failed with an
// arity mismatch.
func TestImplicitSelfInMutatingMethod(t *testing.T) {
	got, diags := generate(t, "lib", `
struct Cipher {
    var key: [Int]
    func nonce() -> [Int] { return key }
    mutating func next() -> Int {
        let n = nonce()
        key = n
        return n.count
    }
}
`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	i := strings.Index(got, "@$s3lib6CipherV4nextSiyF")
	if i < 0 {
		t.Fatalf("no next() in:\n%s", got)
	}
	body := got[i:]
	call := regexp.MustCompile(`apply %\d+\((%\d+)\) : \$@convention\(method\) \(@guaranteed Cipher\)`).FindStringSubmatch(body)
	if call == nil {
		t.Fatalf("no call of nonce() in:\n%s", body)
	}
	if !regexp.MustCompile(regexp.QuoteMeta(call[1]) + ` = load `).MatchString(body) {
		t.Errorf("nonce() is passed %s, which is not a value loaded from self:\n%s", call[1], body)
	}
}

// generateFiles lowers srcs as the files of one module, checked
// together, and returns the SIL text.
func generateFiles(t *testing.T, module string, srcs ...string) string {
	t.Helper()
	var files []*ast.File
	for i, src := range srcs {
		f := token.NewFile(fmt.Sprintf("file%d.swift", i), []byte(src))
		file, diags := parser.ParseFile(f, 0)
		for _, d := range diags {
			t.Fatalf("parse: %s", d.Print(f))
		}
		files = append(files, file)
	}
	info, checks := analyzer.Check(files)
	for _, d := range checks {
		t.Fatalf("check: %s", d.Message)
	}
	m, gens := Files(module, files, info)
	for _, d := range gens {
		t.Fatalf("gen: %s", d.Message)
	}
	var buf bytes.Buffer
	if err := text.Print(&buf, m); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestImplicitSelfTryCallsExtensionMethod: `try? postTarget()` inside a
// method is `try? self.postTarget()`, wherever the extension declaring
// it is. The try? and try! lowering took any name the checker bound to a
// function for a free function, so it called @$s3lib10postTargetSiyKF,
// a symbol nothing defines, and the program failed to link.
func TestImplicitSelfTryCallsExtensionMethod(t *testing.T) {
	got := generateFiles(t, "lib", `
class Renderer {
    var n = 0
    func draw() {
        n = (try? postTarget()) ?? 1
        n = try! postTarget()
        n = (try? Renderer.defaultTarget()) ?? 2
        n = (try? defaultTarget()) ?? 3
    }
    static func reset() {
        let _ = try? defaultTarget()
    }
}
struct Cipher {
    var key = 0
    mutating func next() -> Int {
        key = (try? advance()) ?? 0
        return (try? peek()) ?? key
    }
}
`, `
extension Renderer {
    func postTarget() throws -> Int { return n }
    static func defaultTarget() throws -> Int { return 4 }
}
extension Cipher {
    mutating func advance() throws -> Int { key += 1; return key }
    func peek() throws -> Int { return key }
}
`)
	for _, bad := range []string{"@$s3lib10postTarget", "@$s3lib13defaultTarget", "@$s3lib7advance", "@$s3lib4peek"} {
		if strings.Contains(got, bad) {
			t.Errorf("a method called without self is named as a free function, %s:\n%s", bad, got)
		}
	}
	for _, fn := range []string{"@$s3lib8RendererC4drawyyF", "@$s3lib8RendererC5resetyyFZ", "@$s3lib6CipherV4nextSiyF"} {
		i := strings.Index(got, "sil hidden [ossa] "+fn)
		if i < 0 {
			t.Fatalf("no %s in:\n%s", fn, got)
		}
		body := got[i:]
		if end := strings.Index(body, "\n}\n"); end >= 0 {
			body = body[:end]
		}
		if !strings.Contains(body, "try_apply") {
			t.Errorf("%s calls nothing that may fail:\n%s", fn, body)
		}
	}
	for _, want := range []string{
		"function_ref @$s3lib8RendererC10postTargetSiyKF : $@convention(method) (@guaranteed Renderer)",
		"function_ref @$s3lib8RendererC13defaultTargetSiyKFZ : $@convention(thin) ()",
		"function_ref @$s3lib6CipherV7advanceSiyKF : $@convention(method) (@inout *Cipher)",
		"function_ref @$s3lib6CipherV4peekSiyKF : $@convention(method) (Cipher)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
}
