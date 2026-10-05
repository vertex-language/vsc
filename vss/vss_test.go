package vss

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/token"
)

func parse(t *testing.T, src string) (*File, []token.Diagnostic) {
	t.Helper()
	return Parse(token.NewFile("a.vss", []byte(src)))
}

func TestHeader(t *testing.T) {
	f, diags := parse(t, "/* ui/kit/button.vss */\n// a comment\npackage kit\n\nimport \"ui/theme\"\nimport (\n  \"a/b\"\n  \"c\"\n)\n.button { color: red }\n")
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if f.Package != "kit" {
		t.Errorf("package %q", f.Package)
	}
	var paths []string
	for _, i := range f.Imports {
		paths = append(paths, i.Path)
	}
	if strings.Join(paths, ",") != "ui/theme,a/b,c" {
		t.Errorf("imports %v", paths)
	}
	if strings.TrimSpace(f.Body) != ".button { color: red }" {
		t.Errorf("body %q", f.Body)
	}
}

func TestHeaderErrors(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{".x { }", "begins with its package"},
		{"package\n.x {}", "expected the package's name"},
		{"package kit\nimport ui/theme\n", "expected an import path"},
		{"package kit\n.x { color: red\n", "'{' is never closed"},
		{"package kit\n.x { } }", "'}' closes nothing"},
		{"package kit\n.x { content: \"a }\n", "unterminated string"},
		{"package kit\n/* open", "unterminated comment"},
	} {
		_, diags := parse(t, c.src)
		found := false
		for _, d := range diags {
			if strings.Contains(d.Message, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: want %q, got %v", c.src, c.want, diags)
		}
	}
}

func TestEmit(t *testing.T) {
	f, _ := parse(t, `package kit
@import "x.css";
@font-face { font-family: K; src: url(k.ttf) }
:root { --kit-radius: 6px }
html, body { margin: 0 }
.button { border-radius: var(--kit-radius); &:hover { color: red } }
body.dark .button { color: white }
@media (max-width: 400px) { .button { padding: 2px } }
.label::after { content: "}" }
`)
	css := Emit("kit", []*File{f})
	want := `@import "x.css";
@font-face { font-family: K; src: url(k.ttf) }
@layer kit {
:root { --kit-radius: 6px }
html, body { margin: 0 }
@scope ([data-p="kit"]) to ([data-p]:not([data-p="kit"])) {
.button { border-radius: var(--kit-radius); &:hover { color: red } }
body.dark .button { color: white }
@media (max-width: 400px) { .button { padding: 2px } }
.label::after { content: "}" }
}
}
`
	if css != want {
		t.Errorf("got:\n%s\nwant:\n%s", css, want)
	}
}

func TestSource(t *testing.T) {
	src := Source("kit", "a \"b\" \\(c)\n", []string{"ui/theme", "ui/base"}, []Token{{Name: "--kit-accent", Syntax: "<color>"}, {Name: "--kit-text-color", Syntax: "*"}})
	for _, want := range []string{
		"package kit\n",
		`import __vss_component "ui/component"`,
		`import __vss_after0 "ui/base"`,
		`import __vss_after1 "ui/theme"`,
		`public let __vssSheet = __vss_component.Sheet(package: "kit", css: "a \"b\" \\(c)\n", after: [__vss_after0.__vssSheet, __vss_after1.__vssSheet])`,
		`public static let Accent = __vss_component.Token(name: "--kit-accent", syntax: "<color>")`,
		`public static let TextColor = __vss_component.Token(name: "--kit-text-color", syntax: "*")`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %s in\n%s", want, src)
		}
	}
}

func TestRegistered(t *testing.T) {
	f, _ := parse(t, "package kit\n@property --kit-accent { syntax: \"<color>\"; inherits: true; initial-value: blue }\n@property --kit-any { inherits: false }\n.x {}\n")
	got := Registered([]*File{f})
	if len(got) != 2 || got[0] != (Token{"--kit-accent", "<color>"}) || got[1] != (Token{"--kit-any", "*"}) {
		t.Errorf("registered: %v", got)
	}
}

func TestEmitConditionalDocumentRules(t *testing.T) {
	f, _ := parse(t, "package main\n@media (prefers-color-scheme: dark) { body { color: white } .card { color: gray } }\n")
	css := Emit("main", []*File{f})
	want := "@layer main {\n@media (prefers-color-scheme: dark) {\nbody { color: white }\n}\n@scope ([data-p=\"main\"]) to ([data-p]:not([data-p=\"main\"])) {\n@media (prefers-color-scheme: dark) {\n.card { color: gray }\n}\n}\n}\n"
	if css != want {
		t.Errorf("got:\n%s\nwant:\n%s", css, want)
	}
}
