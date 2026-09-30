package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
)

// The markup grammar is JSX as TypeScript parses it in a .tsx file
// (proposed_vsx.md §6), so TypeScript is the oracle for the part both
// grammars share. Each snippet is compiled by tsc with `--jsx react`, the
// output is run under node with an `h` that records what it is given,
// and that is compared with the same record made from this parser's
// tree. Text is compared as markup means it -- whitespace folded, entities
// decoded -- so JSX's text rules are held to tsc's reading too.
//
// The code inside braces is Vertex in general; the snippets keep it to
// names, and the record says only that code was there. The test needs
// node and tsc on PATH, and skips without them.

var tsxSnippets = []string{
	`<b/>`,
	`<b></b>`,
	`<p>Hello</p>`,
	`<p>Hello {name}!</p>`,
	`<a href="/x" title='t'>home</a>`,
	`<input type="checkbox" checked disabled={n} />`,
	`<app.Window title="T"><Counter start={n} /></app.Window>`,
	`<li class:done={d} data-p="m" aria-label="x"/>`,
	`<>one<i/>two</>`,
	`<div {...attrs} id="x"/>`,
	`<Card header={<h2>Hi</h2>}>body</Card>`,
	`<p>{/* a comment */}</p>`,
	`<p>{}</p>`,
	"<ul>\n    <li>a</li>\n    <li>b</li>\n</ul>",
	"<p>\n  two words\n  on lines  \n</p>",
	"<p>\n  two words\n  on lines  {name}!\n  a &lt; b &amp; c {/* c */}{}\n</p>",
	`<p>a &lt; b &amp; c &#65;&#x42; &quot;q&quot; &apos;</p>`,
	`<p>it's "quoted"</p>`,
	`<a><b><c><d/></c></b></a>`,
	`<p>{a}{b} {c}</p>`,
	`<x.y.z a="1" />`,
	`<T k={n}>{t}</T>`,
	`<p>{<i>nested</i>}</p>`,
	"<p>   leading and trailing   </p>",
	"<p>\n\n   \n</p>",
	"<p>a\n\n\nb</p>",
}

// The names the snippets use as code, declared for tsc and made markers
// for node.
var tsxNames = []string{"name", "n", "d", "attrs", "a", "b", "c", "t"}

// The components the snippets name, which tsc compiles to references.
var tsxComponents = []string{"Card", "Counter", "T", "app", "x"}

// recordVSX is the record of the first element in a .vsx snippet.
func recordVSX(t *testing.T, snippet string) string {
	t.Helper()
	f := token.NewFile("s.vsx", []byte("let x = "+snippet))
	file, diags := ParseFile(f, 0)
	for _, d := range diags {
		t.Errorf("%s: %s", snippet, d.Print(f))
	}
	var el *ast.MarkupElement
	ast.Inspect(file, func(n ast.Node) bool {
		if e, ok := n.(*ast.MarkupElement); ok && el == nil {
			el = e
		}
		return el == nil
	})
	if el == nil {
		t.Fatalf("%s: no markup", snippet)
	}
	var b strings.Builder
	var elem func(e *ast.MarkupElement)
	code := func(c *ast.MarkupCode) {
		if single := codeElement(c); single != nil {
			elem(single)
		} else {
			b.WriteString("{}")
		}
	}
	elem = func(e *ast.MarkupElement) {
		b.WriteString("<")
		if e.Name != nil {
			b.WriteString(e.Name.Text(f))
		}
		for _, a := range e.Attrs {
			switch a := a.(type) {
			case *ast.MarkupSpread:
				b.WriteString(" {...}")
			case *ast.MarkupAttribute:
				b.WriteString(" " + a.Name.Text(f))
				switch v := a.Value.(type) {
				case *ast.MarkupString:
					b.WriteString("=" + fmt.Sprintf("%q", v.Value(f)))
				case *ast.MarkupCode:
					b.WriteString("=")
					code(v)
				}
			}
		}
		b.WriteString(">")
		for _, c := range e.Children {
			switch c := c.(type) {
			case *ast.MarkupText:
				if s := c.Value(f); s != "" {
					b.WriteString(fmt.Sprintf("%q", s))
				}
			case *ast.MarkupCode:
				// Empty braces and comments are nothing.
				if len(c.Body.Stmts) > 0 || c.Body.Sig != nil {
					code(c)
				}
			case *ast.MarkupElement:
				elem(c)
			}
		}
		b.WriteString("</>")
	}
	elem(el)
	return b.String()
}

// codeElement is the element a `{…}` holds alone, or nil.
func codeElement(c *ast.MarkupCode) *ast.MarkupElement {
	if len(c.Body.Stmts) != 1 {
		return nil
	}
	if s, ok := c.Body.Stmts[0].(*ast.ExprStmt); ok {
		if e, ok := s.X.(*ast.MarkupElement); ok {
			return e
		}
	}
	return nil
}

// The same record, made from what tsc's output passes h. Strings are
// quoted as Go's %q quotes them.
const tsxRecorder = `
const M = { marker: true };
for (const n of NAMES) globalThis[n] = M;
globalThis.attrs = { "...": M };
globalThis.F = "fragment";
// A component is a reference; here it is its own name, dotted as written.
const named = (path) => new Proxy({}, {
  get: (_, k) => k === Symbol.toPrimitive || k === "toString" ? () => path : named(path + "." + String(k)),
});
for (const c of COMPONENTS) globalThis[c] = named(c);
globalThis.h = (tag, props, ...children) => ({ tag, props, children });
const q = (s) => JSON.stringify(s).replace(/\\u00a0/g, " ");
const rec = (v) => {
  if (v === M) return "{}";
  let s = "<" + (v.tag === "fragment" ? "" : String(v.tag));
  for (const [k, p] of Object.entries(v.props || {})) {
    if (k === "...") { s += " {...}"; continue; }
    s += " " + k;
    if (p === true) continue;
    if (typeof p === "string") s += "=" + q(p);
    else s += "=" + rec(p);
  }
  s += ">";
  for (const c of v.children) s += typeof c === "string" ? q(c) : rec(c);
  return s + "</>";
};
const out = [];
for (let i = 0; i < COUNT; i++) out.push(rec(require(DIR + "/s" + i + ".js").__v));
process.stdout.write(JSON.stringify(out));
`

func TestTSXOracle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on PATH")
	}
	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Skip("no tsc on PATH (npm i -g typescript)")
	}
	dir := t.TempDir()
	decl := "declare function h(...a: any[]): any; declare var F: any; declare var " +
		strings.Join(append(tsxNames, tsxComponents...), ": any, ") + ": any;\n"
	var files []string
	for i, s := range tsxSnippets {
		name := filepath.Join(dir, fmt.Sprintf("s%d.tsx", i))
		if err := os.WriteFile(name, []byte(decl+"export const __v = "+s+";\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, name)
	}
	out := filepath.Join(dir, "out")
	args := append([]string{"--jsx", "react", "--jsxFactory", "h", "--jsxFragmentFactory", "F",
		"--target", "es2020", "--module", "commonjs", "--outDir", out}, files...)
	// The snippets are not typed -- there is no JSX namespace -- so tsc
	// reports type errors and emits all the same. A syntax error is what
	// matters, and it shows as a missing or different record.
	msgs, _ := exec.Command(tsc, args...).CombinedOutput()
	for _, line := range strings.Split(string(msgs), "\n") {
		if strings.Contains(line, "error TS1") { // TS1xxx are syntax errors
			t.Errorf("tsc: %s", line)
		}
	}
	names, _ := json.Marshal(tsxNames)
	components, _ := json.Marshal(tsxComponents)
	script := strings.NewReplacer("NAMES", string(names), "COMPONENTS", string(components), "COUNT", fmt.Sprint(len(tsxSnippets)),
		"DIR", fmt.Sprintf("%q", out)).Replace(tsxRecorder)
	cmd := exec.Command(node, "-e", script)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	res, err := cmd.Output()
	if err != nil {
		t.Fatalf("node: %v\n%s\ntsc said:\n%s", err, stderr.String(), msgs)
	}
	var records []string
	if err := json.Unmarshal(res, &records); err != nil {
		t.Fatalf("node's answer: %v\n%s", err, res)
	}
	for i, s := range tsxSnippets {
		if got, want := recordVSX(t, s), records[i]; got != want {
			t.Errorf("%q\n vsc: %s\n tsx: %s", s, got, want)
		}
	}
}
