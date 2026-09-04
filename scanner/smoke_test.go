package scanner

import (
	"fmt"
	"testing"

	"github.com/vertex-language/vsc/token"
)

func TestSmoke(t *testing.T) {
	srcs := []string{
		`let x = 1_000 + 0x1.8p3 * 0b1010`,
		`print("hi \(name.uppercased()) and \(a + b)!")`,
		`#"raw \#(x) \n"#`,
		"let s = \"\"\"\n  hello\n  \"\"\"",
		`let r = /[a-z]+\/x/ ; let q = a / b; let p = 1/2`,
		"func f<T: P>(_ x: T?) -> [T] { x.map { $0 } }",
		"if #available(iOS 15, *) { x?.y!.z }",
		"@MainActor class `class`: Base {}",
		"a ..< b, c...d, x!, .foo, &y, i += 1",
		"prefix operator √; let r = √16.0",
		"postfix operator °; let a = 90.0°",
		"infix operator .+.; let sum = 1 .+. 2",
		"infix operator **; let pow = 2.0 ** 8.0",
	}
	for _, src := range srcs {
		f := token.NewFile("t.vs", []byte(src))
		toks, diags := Scan(f, 0)
		for _, d := range diags {
			t.Errorf("unexpected diag for %q: %s", src, d.Print(f))
		}
		var out string
		for _, tk := range toks {
			if tk.Kind == token.EOF {
				break
			}
			out += fmt.Sprintf("%s(%s) ", tk.Kind, f.Slice(tk.Pos, tk.End))
		}
		t.Logf("SRC %s\n  %s", src, out)
	}
}
