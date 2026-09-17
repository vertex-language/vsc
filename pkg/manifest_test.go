package pkg

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/token"
)

// TestManifestsMatchSwiftPM reads every package in tests/packages and
// compares what vsc makes of its manifest with what SwiftPM does:
// `swift package dump-package` runs the manifest and prints the Package
// it built, so it is the answer and cannot drift.
//
// Both sides are projected onto the parts vsc builds from -- targets,
// their files and settings, products, platforms, standards -- because the
// dump carries bookkeeping (identities, absolute paths, trait sets) that
// is SwiftPM's and not the manifest's.
func TestManifestsMatchSwiftPM(t *testing.T) {
	swift, err := exec.LookPath("swift")
	if err != nil {
		t.Skip("no swift on PATH")
	}
	dirs, _ := filepath.Glob("../tests/packages/*")
	ran := 0
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, ManifestName)); err != nil {
			continue
		}
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			m, diags, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range diags {
				if d.Severity == token.Error {
					t.Fatalf("vsc refused the manifest: %s", d.Message)
				}
			}
			cmd := exec.Command(swift, "package", "dump-package",
				"--package-path", dir, "--scratch-path", t.TempDir())
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("dump-package: %v", err)
			}
			var dump map[string]any
			if err := json.Unmarshal(out, &dump); err != nil {
				t.Fatal(err)
			}
			want, err := fromDump(dump)
			if err != nil {
				t.Fatal(err)
			}
			got := canonical(m)
			gotJSON, _ := json.MarshalIndent(got, "", "  ")
			wantJSON, _ := json.MarshalIndent(want, "", "  ")
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("vsc:\n%s\nSwiftPM:\n%s", gotJSON, wantJSON)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no packages in tests/packages")
	}
}

// canon is a manifest reduced to what both sides can say the same way.
type canon struct {
	ToolsVersion string
	Name         string
	Platforms    []string
	Products     []string
	Dependencies []string
	Targets      []canonTarget
	CStandard    string
	CXXStandard  string
}

type canonTarget struct {
	Name              string
	Kind              string
	Dependencies      []string
	Path              string
	PublicHeadersPath string
	Sources           []string
	Exclude           []string
	Settings          []string
}

func canonical(m *Manifest) canon {
	c := canon{
		ToolsVersion: fullVersion(m.ToolsVersion),
		Name:         m.Name,
		CStandard:    m.CLanguageStandard,
		CXXStandard:  m.CXXLanguageStandard,
	}
	for _, p := range m.Platforms {
		c.Platforms = append(c.Platforms, p.Name+" "+p.Version)
	}
	for _, p := range m.Products {
		kind := p.Kind
		if p.Kind == "library" {
			kind += "(" + p.Library + ")"
		}
		c.Products = append(c.Products, fmt.Sprintf("%s %s %v", kind, p.Name, p.Targets))
	}
	for _, d := range m.Dependencies {
		c.Dependencies = append(c.Dependencies, "path "+d.Path+" url "+d.URL)
	}
	for _, t := range m.Targets {
		ct := canonTarget{Name: t.Name, Kind: t.Kind, Exclude: t.Exclude}
		if t.Path != nil {
			ct.Path = *t.Path
		}
		if t.PublicHeadersPath != nil {
			ct.PublicHeadersPath = *t.PublicHeadersPath
		}
		if t.HasSources {
			ct.Sources = append([]string{}, t.Sources...)
		}
		for _, d := range t.Dependencies {
			s := d.Kind + " " + d.Name
			if d.Package != "" {
				s += " in " + d.Package
			}
			ct.Dependencies = append(ct.Dependencies, s)
		}
		for _, s := range t.Settings {
			ct.Settings = append(ct.Settings, s.Tool+" "+s.Kind+" "+strings.Join(s.Values, ","))
		}
		c.Targets = append(c.Targets, ct)
	}
	return c
}

// fullVersion is "6.0" as SwiftPM prints it, "6.0.0".
func fullVersion(v string) string {
	for strings.Count(v, ".") < 2 {
		v += ".0"
	}
	return v
}

// fromDump projects dump-package's JSON the same way.
func fromDump(d map[string]any) (canon, error) {
	c := canon{Name: str(d["name"]), CStandard: str(d["cLanguageStandard"]), CXXStandard: str(d["cxxLanguageStandard"])}
	if tv, ok := d["toolsVersion"].(map[string]any); ok {
		c.ToolsVersion = str(tv["_version"])
	}
	for _, p := range objects(d["platforms"]) {
		c.Platforms = append(c.Platforms, str(p["platformName"])+" "+str(p["version"]))
	}
	for _, p := range objects(d["products"]) {
		kind := ""
		for k, v := range object(p["type"]) {
			kind = k
			if k == "library" {
				if list, ok := v.([]any); ok && len(list) > 0 {
					kind += "(" + str(list[0]) + ")"
				}
			}
		}
		c.Products = append(c.Products, fmt.Sprintf("%s %s %v", kind, str(p["name"]), stringList(p["targets"])))
	}
	if deps := objects(d["dependencies"]); len(deps) > 0 {
		return c, fmt.Errorf("the projection does not read package dependencies yet: extend fromDump")
	}
	for _, t := range objects(d["targets"]) {
		ct := canonTarget{
			Name:              str(t["name"]),
			Kind:              str(t["type"]),
			Path:              str(t["path"]),
			PublicHeadersPath: str(t["publicHeadersPath"]),
			Exclude:           stringList(t["exclude"]),
		}
		if t["sources"] != nil {
			ct.Sources = append([]string{}, stringList(t["sources"])...)
		}
		for _, dep := range objects(t["dependencies"]) {
			for k, v := range dep {
				list, _ := v.([]any)
				s := k + " " + str(first(list, 0))
				if k == "product" && str(first(list, 1)) != "" {
					s += " in " + str(first(list, 1))
				}
				ct.Dependencies = append(ct.Dependencies, s)
			}
		}
		for _, s := range objects(t["settings"]) {
			for kind, v := range object(s["kind"]) {
				var values []string
				switch x := object(v)["_0"].(type) {
				case string:
					values = []string{x}
				case []any:
					values = stringList(x)
				}
				ct.Settings = append(ct.Settings, str(s["tool"])+" "+kind+" "+joinStrings(values))
			}
		}
		c.Targets = append(c.Targets, ct)
	}
	return c, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func objects(v any) []map[string]any {
	list, _ := v.([]any)
	var out []map[string]any
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringList(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, x := range list {
		out = append(out, str(x))
	}
	return out
}

func first(list []any, i int) any {
	if i < len(list) {
		return list[i]
	}
	return nil
}

func joinStrings(values []string) string {
	out := ""
	for i, v := range values {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}

// TestManifestReadsBindingsAndConventions: a target bound to a name and
// used in the package, the arguments that override SwiftPM's layout, and a
// product dependency with a condition.
func TestManifestReadsBindingsAndConventions(t *testing.T) {
	src := `// swift-tools-version: 5.9
import PackageDescription

let core: Target = .target(
    name: "Core",
    path: "src/core",
    exclude: ["old"],
    sources: ["a.c", "b.c"],
    publicHeadersPath: "inc",
    cSettings: [.define("DEBUG_LOG"), .unsafeFlags(["-Wall"], .when(platforms: [.linux]))]
)

let package = Package(
    name: "P",
    dependencies: [.package(path: "../Other"), .package(url: "https://example.com/y.git", from: "1.2.0")],
    targets: [
        core,
        .testTarget(name: "CoreTests", dependencies: ["Core", .product(name: "Y", package: "y")]),
    ],
    cxxLanguageStandard: .gnucxx20
)
`
	m, diags := Parse("Package.swift", []byte(src))
	for _, d := range diags {
		t.Fatalf("refused: %s", d.Message)
	}
	if m.ToolsVersion != "5.9" || m.Name != "P" || m.CXXLanguageStandard != "gnu++20" {
		t.Errorf("package = %q %q %q", m.ToolsVersion, m.Name, m.CXXLanguageStandard)
	}
	if len(m.Dependencies) != 2 || m.Dependencies[0].Path != "../Other" ||
		m.Dependencies[1].Requirement.Kind != "from" || m.Dependencies[1].Requirement.Values[0] != "1.2.0" {
		t.Errorf("dependencies = %+v", m.Dependencies)
	}
	if len(m.Targets) != 2 {
		t.Fatalf("targets = %+v", m.Targets)
	}
	core := m.Targets[0]
	if core.Path == nil || *core.Path != "src/core" || core.PublicHeadersPath == nil || *core.PublicHeadersPath != "inc" ||
		!core.HasSources || len(core.Sources) != 2 || len(core.Exclude) != 1 {
		t.Errorf("core = %+v", core)
	}
	if len(core.Settings) != 2 || core.Settings[1].Condition == nil || core.Settings[1].Condition.Platforms[0] != "linux" {
		t.Errorf("core settings = %+v", core.Settings)
	}
	tests := m.Targets[1]
	if tests.Kind != TargetTest || len(tests.Dependencies) != 2 || tests.Dependencies[1].Package != "y" {
		t.Errorf("tests = %+v", tests)
	}
}

// TestManifestRefusesWhatItDoesNotRun: what a manifest can only do by
// running is reported where it is written.
func TestManifestRefusesWhatItDoesNotRun(t *testing.T) {
	const head = "// swift-tools-version: 6.0\nimport PackageDescription\n"
	cases := []struct{ name, src, want string }{
		{"a statement", head + "let package = Package(name: \"A\")\npackage.name = \"B\"\n", "does not run its statements"},
		{"an interpolation", head + "let n = 1\nlet package = Package(name: \"A\\(n)\")\n", "cannot interpolate"},
		{"no tools version", "import PackageDescription\nlet package = Package(name: \"A\")\n", "swift-tools-version"},
		{"no package", head + "let other = Package(name: \"A\")\n", "binds no 'package'"},
		{"an unknown argument", head + "let package = Package(name: \"A\", targets: [.target(name: \"B\", flavor: \"x\")])\n", "no argument 'flavor'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags := Parse("Package.swift", []byte(c.src))
			var msgs []string
			for _, d := range diags {
				msgs = append(msgs, d.Message)
			}
			sort.Strings(msgs)
			if !containsAny(msgs, c.want) {
				t.Errorf("want a diagnostic containing %q, got %q", c.want, msgs)
			}
		})
	}
}

func containsAny(msgs []string, want string) bool {
	for _, m := range msgs {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}
