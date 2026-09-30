package vss

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/token"
)

func check(t *testing.T, src string, main bool) []token.Diagnostic {
	t.Helper()
	f, diags := parse(t, src)
	if len(diags) != 0 {
		t.Fatalf("parse: %v", diags)
	}
	return Check(f, main, nil)
}

func TestCheck(t *testing.T) {
	src := `package kit
:root { --kit-accent: blue; color: red }
.card {
    colr: red;
    padding: 4px;
    transition: color 1s;
    &:hover { bakground: none }
    a:hover { color: blue }
    --local: 1;
    @media (max-width: 10px) { margn: 0 }
}
@font-face { font-family: K; src: url(k.ttf) }
@keyframes spin { from { transform: none } }
`
	var got []string
	for _, d := range check(t, src, false) {
		got = append(got, d.Severity.String()+": "+d.Message)
	}
	want := []string{
		"error: only package main styles the document; a library may set custom properties here, not 'color'",
		"error: unknown property 'colr'; did you mean 'color'?",
		"warning: the engine does not apply 'transition' yet: it will have no effect",
		"error: unknown property 'bakground'; did you mean 'background'?",
		"error: unknown property 'margn'; did you mean 'margin'?",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if diags := check(t, "package main\n:root { color: red }\nbody { margin: 0 }\n", true); len(diags) != 0 {
		t.Errorf("package main may style the document: %v", diags)
	}
}

// The embedded names are what the engine says now: web/cmd/css-names,
// where the web repository and vsc are beside this one.
func TestPropertiesAreTheEngines(t *testing.T) {
	web := filepath.Join("..", "..", "web")
	if _, err := os.Stat(filepath.Join(web, "cmd", "css-names")); err != nil {
		t.Skip("no web repository beside vsc")
	}
	vsc, err := exec.LookPath("vsc")
	if err != nil {
		t.Skip("no vsc on PATH")
	}
	cmd := exec.Command(vsc, "run", "css-names")
	cmd.Dir = web
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("vsc run css-names: %v", err)
	}
	var now, embedded map[string][]string
	if err := json.Unmarshal(out, &now); err != nil {
		t.Fatalf("css-names printed: %v", err)
	}
	if err := json.Unmarshal(propertiesJSON, &embedded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(now, embedded) {
		t.Errorf("vss/properties.json is not what the engine says: run `vsc run css-names > ../vsc/vss/properties.json` in web")
	}
}

func TestTokens(t *testing.T) {
	kit, _ := parse(t, `package kit
@property --kit-accent { syntax: "<color>"; inherits: true; initial-value: blue }
@property --radius { syntax: "<length>"; inherits: true; initial-value: 6px }
:root { --kit-radius: 6px }
.card { --kit-inner: 1px; border-radius: var(--kit-radius); padding: var(--kit-inner); color: var(--kit-acent) }
`)
	exported := Tokens([]*File{kit})
	if !exported["--kit-accent"] || !exported["--kit-radius"] || exported["--kit-inner"] {
		t.Errorf("exported tokens: %v", exported)
	}
	var got []string
	for _, d := range Check(kit, false, map[string]map[string]bool{"kit": Declared([]*File{kit})}) {
		got = append(got, d.Message)
	}
	want := []string{
		"a token of package kit is named --kit-…: custom properties share the page, so a library's carry its name ('--radius')",
		"package kit declares no token --kit-acent; did you mean --kit-accent?",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	app, _ := parse(t, `package main
:root { --kit-accent: black; --kit-acent: red; --mine: 1 }
.x { color: var(--kit-radius); width: var(--kit-inner) }
`)
	got = nil
	for _, d := range Check(app, true, map[string]map[string]bool{"kit": exported, "main": Declared([]*File{app})}) {
		got = append(got, d.Message)
	}
	want = []string{
		"package kit declares no token --kit-acent; did you mean --kit-accent?",
		"package kit declares no token --kit-inner",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("app: got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
