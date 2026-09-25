// Package pkg parses and evaluates SwiftPM Package.swift manifests.
package pkg

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// ManifestName is the file a SwiftPM package is rooted at. A Vertex
// package has no manifest: it is a folder (see Folder), and its module's
// requirements, where it states any, are in a vs.mod (see ModFile).
const ManifestName = "Package.swift"

// FindManifest is the Package.swift of the package rooted at dir, and
// whether there is one.
func FindManifest(dir string) (string, bool) {
	path := filepath.Join(dir, ManifestName)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, true
	}
	return "", false
}

// A Manifest is a Package.swift as SwiftPM understands it: the Package
// value the manifest builds, which `swift package dump-package` prints.
type Manifest struct {
	// ToolsVersion is the first line's `// swift-tools-version:`.
	ToolsVersion        string
	Name                string
	Platforms           []Platform
	Products            []Product
	Dependencies        []PackageDependency
	Targets             []Target
	CLanguageStandard   string
	CXXLanguageStandard string
}

// A Platform is a minimum deployment version: `.macOS(.v13)`.
type Platform struct {
	Name    string // "macos", "ios", ...: SwiftPM's platform names
	Version string // "13.0"
}

// A Product is what the package offers the packages that depend on it.
type Product struct {
	Name string
	// Kind is "executable" or "library".
	Kind string
	// Library is a library's linkage: "automatic", "static" or "dynamic".
	Library string
	Targets []string
}

// A PackageDependency is another package, by path or by URL.
type PackageDependency struct {
	// Name is the name the manifest gave it, where it gave one.
	Name string
	Path string
	URL  string
	// Requirement is a URL dependency's version rule: its kind ("from",
	// "exact", "branch", "revision", "range") and the values it names.
	Requirement Requirement
}

// A Requirement is which versions of a URL dependency are acceptable.
type Requirement struct {
	Kind   string
	Values []string
}

// Target kinds, as dump-package spells them.
const (
	TargetRegular    = "regular"
	TargetExecutable = "executable"
	TargetTest       = "test"
	TargetSystem     = "system"
	TargetBinary     = "binary"
	TargetPlugin     = "plugin"
	TargetMacro      = "macro"
)

// A Target is one module of the package.
type Target struct {
	Name         string
	Kind         string
	Dependencies []TargetDependency
	// Path, Sources and PublicHeadersPath are nil where the manifest
	// left SwiftPM's conventions to decide them.
	Path              *string
	Exclude           []string
	Sources           []string
	HasSources        bool
	PublicHeadersPath *string
	Settings          []Setting
	PkgConfig         *string
	URL               string
	Checksum          string
}

// A TargetDependency is what a target needs built first.
type TargetDependency struct {
	// Kind is "byName" for a bare string, "target" or "product".
	Kind      string
	Name      string
	Package   string
	Condition *Condition
}

// A Setting is one build setting for one tool.
type Setting struct {
	// Tool is "c", "cxx", "swift" or "linker".
	Tool string
	// Kind is the setting's name: "define", "headerSearchPath",
	// "unsafeFlags", "linkedLibrary", "interoperabilityMode", ...
	Kind      string
	Values    []string
	Condition *Condition
}

// A Condition limits a setting or a dependency to some platforms or one
// configuration.
type Condition struct {
	Platforms []string
	Config    string
}

// Load reads the manifest of the package rooted at dir.
func Load(dir string) (*Manifest, []token.Diagnostic, error) {
	path, ok := FindManifest(dir)
	if !ok {
		path = filepath.Join(dir, ManifestName)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	m, diags := Parse(path, src)
	return m, diags, nil
}

// Parse parses and statically evaluates a Package.swift manifest.
func Parse(name string, src []byte) (*Manifest, []token.Diagnostic) {
	f := token.NewFile(name, src)
	file, diags := parser.ParseFile(f, 0)
	e := &evaluator{file: f, lets: map[string]ast.Expr{}}
	e.diags = append(e.diags, diags...)
	if errorsIn(diags) {
		return nil, e.diags
	}
	m := &Manifest{}
	m.ToolsVersion = toolsVersion(src)
	if m.ToolsVersion == "" {
		e.errorAt(file, "the manifest's first line must be '// swift-tools-version: X.Y', which says which PackageDescription it is written against")
	}

	var pkg ast.Expr
	for _, st := range file.Stmts {
		switch s := st.(type) {
		case *ast.DeclStmt:
			switch d := s.D.(type) {
			case *ast.ImportDecl:
				continue
			case *ast.VarDecl:
				for _, b := range d.Bindings {
					pat := b.Pat
					// `let core: Target = …` names the type, which the
					// value's constructor already says.
					if tp, typed := pat.(*ast.TypedPattern); typed {
						pat = tp.Pat
					}
					id, ok := pat.(*ast.IdentPattern)
					if !ok || id.Name == nil || b.Value == nil {
						e.errorAt(b, "a manifest binding is read as 'let name = value'")
						continue
					}
					name := id.Name.Text(f)
					e.lets[name] = b.Value
					if name == "package" {
						pkg = b.Value
					}
				}
				continue
			}
			e.errorAt(s, "a manifest declares its package: this declaration is not one vsc reads")
		case *ast.EmptyStmt:
		default:
			e.errorAt(st, "vsc reads a manifest's declarations and does not run its statements: "+
				"build the Package(…) in the 'let package' binding")
		}
	}
	if pkg == nil {
		e.errorAt(file, "the manifest binds no 'package': write 'let package = Package(name: …)'")
		return nil, e.diags
	}
	e.pkg(m, pkg)
	if errorsIn(e.diags) {
		return nil, e.diags
	}
	return m, e.diags
}

var toolsVersionLine = regexp.MustCompile(`^\s*//\s*swift-tools-version\s*:\s*([0-9]+(?:\.[0-9]+){0,2})`)

func toolsVersion(src []byte) string {
	line := string(src)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if m := toolsVersionLine.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

func errorsIn(diags []token.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == token.Error {
			return true
		}
	}
	return false
}

// An evaluator reads PackageDescription's values out of the syntax.
type evaluator struct {
	file  *token.File
	diags []token.Diagnostic
	lets  map[string]ast.Expr
	depth int
}

func (e *evaluator) errorAt(n ast.Node, msg string) {
	e.diags = append(e.diags, token.Diagnostic{
		Pos: n.Pos(), End: n.End(), Severity: token.Error, File: e.file, Message: msg,
	})
}

func (e *evaluator) text(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return id.Text(e.file)
}

// resolve follows a name to the value a `let` bound it to.
func (e *evaluator) resolve(x ast.Expr) ast.Expr {
	for i := 0; i < 32; i++ {
		switch v := x.(type) {
		case *ast.ParenExpr:
			x = v.X
			continue
		case *ast.IdentExpr:
			if bound, ok := e.lets[e.text(v.Name)]; ok {
				x = bound
				continue
			}
		}
		return x
	}
	return x
}

// A call is PackageDescription's `.name(label: value, …)`, `Type(…)` or
// `Type.name(…)`.
type call struct {
	node ast.Node
	name string
	args []*ast.CallArg
}

func (e *evaluator) call(x ast.Expr) (call, bool) {
	x = e.resolve(x)
	ce, ok := x.(*ast.CallExpr)
	if !ok {
		return call{}, false
	}
	var name string
	switch fn := ce.Fun.(type) {
	case *ast.ImplicitMemberExpr:
		name = e.text(fn.Name)
	case *ast.MemberExpr:
		name = e.text(fn.Name)
	case *ast.IdentExpr:
		name = e.text(fn.Name)
	default:
		return call{}, false
	}
	c := call{node: ce, name: name}
	if ce.Args != nil {
		c.args = ce.Args.Args
	}
	if len(ce.Trailing) > 0 {
		e.errorAt(ce, "a closure in a manifest is not something vsc evaluates")
	}
	return c, true
}

func (e *evaluator) label(a *ast.CallArg) string { return e.text(a.Label) }

// unknown reports an argument a PackageDescription call does not take, or
// that vsc does not read yet.
func (e *evaluator) unknown(c call, a *ast.CallArg) {
	l := e.label(a)
	if l == "" {
		l = "_"
	}
	e.errorAt(a, "'"+c.name+"' has no argument '"+l+"' that vsc reads")
}

// str is a string literal with nothing interpolated.
func (e *evaluator) str(x ast.Expr) (string, bool) {
	x = e.resolve(x)
	lit, ok := x.(*ast.StringLit)
	if !ok {
		e.errorAt(x, "expected a string literal")
		return "", false
	}
	var b strings.Builder
	for _, seg := range lit.Segments {
		t, ok := seg.(*ast.StringText)
		if !ok {
			e.errorAt(seg, "a string in a manifest cannot interpolate")
			return "", false
		}
		b.WriteString(unescape(string(e.file.Slice(t.Pos(), t.End()))))
	}
	return b.String(), true
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '0':
			b.WriteByte(0)
		case 'u':
			if end := strings.IndexByte(s[i:], '}'); i+1 < len(s) && s[i+1] == '{' && end > 0 {
				if r, err := strconv.ParseUint(s[i+2:i+end], 16, 32); err == nil {
					b.WriteRune(rune(r))
					i += end
					continue
				}
			}
			b.WriteString(`\u`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// list is an array literal's elements.
func (e *evaluator) list(x ast.Expr) ([]ast.Expr, bool) {
	x = e.resolve(x)
	arr, ok := x.(*ast.ArrayLit)
	if !ok {
		e.errorAt(x, "expected an array literal")
		return nil, false
	}
	return arr.Items, true
}

func (e *evaluator) strs(x ast.Expr) ([]string, bool) {
	items, ok := e.list(x)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := e.str(it)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func (e *evaluator) isNil(x ast.Expr) bool {
	lit, ok := e.resolve(x).(*ast.BasicLit)
	return ok && lit.Kind == token.NIL
}

func (e *evaluator) boolean(x ast.Expr) (bool, bool) {
	if lit, ok := e.resolve(x).(*ast.BasicLit); ok {
		switch lit.Kind {
		case token.TRUE:
			return true, true
		case token.FALSE:
			return false, true
		}
	}
	e.errorAt(x, "expected true or false")
	return false, false
}

// caseName is an enum case written `.name` or `Type.name`.
func (e *evaluator) caseName(x ast.Expr) (string, bool) {
	switch v := e.resolve(x).(type) {
	case *ast.ImplicitMemberExpr:
		return e.text(v.Name), true
	case *ast.MemberExpr:
		return e.text(v.Name), true
	}
	e.errorAt(x, "expected a case, as in '.name'")
	return "", false
}

// optionalStr is a String? argument: a string, or nil.
func (e *evaluator) optionalStr(x ast.Expr) (*string, bool) {
	if e.isNil(x) {
		return nil, true
	}
	s, ok := e.str(x)
	if !ok {
		return nil, false
	}
	return &s, true
}

// pkg reads `Package(name: …, …)`.
func (e *evaluator) pkg(m *Manifest, x ast.Expr) {
	c, ok := e.call(x)
	if !ok || c.name != "Package" {
		e.errorAt(x, "'package' must be a Package(…)")
		return
	}
	for _, a := range c.args {
		switch e.label(a) {
		case "name":
			m.Name, _ = e.str(a.X)
		case "defaultLocalization", "swiftLanguageVersions", "swiftLanguageModes", "providers", "pkgConfig":
			// Nothing vsc builds differently for these.
		case "platforms":
			if e.isNil(a.X) {
				continue
			}
			items, _ := e.list(a.X)
			for _, it := range items {
				if p, ok := e.platform(it); ok {
					m.Platforms = append(m.Platforms, p)
				}
			}
		case "products":
			items, _ := e.list(a.X)
			for _, it := range items {
				if p, ok := e.product(it); ok {
					m.Products = append(m.Products, p)
				}
			}
		case "dependencies":
			items, _ := e.list(a.X)
			for _, it := range items {
				if d, ok := e.packageDependency(it); ok {
					m.Dependencies = append(m.Dependencies, d)
				}
			}
		case "targets":
			items, _ := e.list(a.X)
			for _, it := range items {
				if t, ok := e.target(it); ok {
					m.Targets = append(m.Targets, t)
				}
			}
		case "cLanguageStandard":
			if !e.isNil(a.X) {
				// A Vertex manifest may name the standard by its spelling.
				if _, isString := e.resolve(a.X).(*ast.StringLit); isString {
					if s, ok := e.str(a.X); ok {
						m.CLanguageStandard = s
					}
				} else if s, ok := e.caseName(a.X); ok {
					m.CLanguageStandard = cStandard(s)
				}
			}
		case "cxxLanguageStandard":
			if !e.isNil(a.X) {
				// "c++20" as well as .cxx20.
				if _, isString := e.resolve(a.X).(*ast.StringLit); isString {
					if s, ok := e.str(a.X); ok {
						m.CXXLanguageStandard = s
					}
				} else if s, ok := e.caseName(a.X); ok {
					m.CXXLanguageStandard = cStandard(s)
				}
			}
		default:
			e.unknown(c, a)
		}
	}
	if m.Name == "" {
		e.errorAt(c.node, "a Package needs a name")
	}
}

// cStandard is a language standard case's raw value: `.cxx17` is "c++17"
// and `.gnucxx20` is "gnu++20", which is the flag clang takes.
func cStandard(name string) string {
	name = strings.Replace(name, "gnucxx", "gnu++", 1)
	return strings.Replace(name, "cxx", "c++", 1)
}

var platformNames = map[string]string{
	"macOS": "macos", "iOS": "ios", "tvOS": "tvos", "watchOS": "watchos",
	"visionOS": "visionos", "macCatalyst": "maccatalyst", "driverKit": "driverkit",
	"linux": "linux", "windows": "windows", "android": "android", "wasi": "wasi",
	"openbsd": "openbsd", "freebsd": "freebsd",
}

// platform reads `.macOS(.v13)` or `.macOS("13.1")`.
func (e *evaluator) platform(x ast.Expr) (Platform, bool) {
	c, ok := e.call(x)
	if !ok || len(c.args) != 1 {
		e.errorAt(x, "expected a platform, as in '.macOS(.v13)'")
		return Platform{}, false
	}
	name, known := platformNames[c.name]
	if !known {
		e.errorAt(x, "'"+c.name+"' is not a platform vsc knows")
		return Platform{}, false
	}
	arg := e.resolve(c.args[0].X)
	if _, isString := arg.(*ast.StringLit); isString {
		v, ok := e.str(arg)
		return Platform{Name: name, Version: v}, ok
	}
	v, ok := e.caseName(arg)
	if !ok || !strings.HasPrefix(v, "v") {
		return Platform{}, false
	}
	// `.v10_15` is "10.15", and `.v13` is "13.0".
	ver := strings.ReplaceAll(v[1:], "_", ".")
	if !strings.Contains(ver, ".") {
		ver += ".0"
	}
	return Platform{Name: name, Version: ver}, true
}

// product reads `.executable(name:targets:)` and `.library(name:type:targets:)`.
func (e *evaluator) product(x ast.Expr) (Product, bool) {
	c, ok := e.call(x)
	if !ok || (c.name != "executable" && c.name != "library") {
		e.errorAt(x, "expected a product: '.executable(name:targets:)' or '.library(name:targets:)'")
		return Product{}, false
	}
	p := Product{Kind: c.name}
	if c.name == "library" {
		p.Library = "automatic"
	}
	for _, a := range c.args {
		switch e.label(a) {
		case "name":
			p.Name, _ = e.str(a.X)
		case "targets":
			p.Targets, _ = e.strs(a.X)
		case "type":
			if c.name == "library" && !e.isNil(a.X) {
				p.Library, _ = e.caseName(a.X)
			} else if c.name != "library" {
				e.unknown(c, a)
			}
		default:
			e.unknown(c, a)
		}
	}
	return p, true
}

// packageDependency reads `.package(path:)` and `.package(url:…)`.
func (e *evaluator) packageDependency(x ast.Expr) (PackageDependency, bool) {
	c, ok := e.call(x)
	if !ok || c.name != "package" {
		e.errorAt(x, "expected a dependency, as in '.package(path: \"../Other\")'")
		return PackageDependency{}, false
	}
	var d PackageDependency
	for _, a := range c.args {
		switch l := e.label(a); l {
		case "name":
			d.Name, _ = e.str(a.X)
		case "path":
			d.Path, _ = e.str(a.X)
		case "url":
			d.URL, _ = e.str(a.X)
		case "from", "exact", "branch", "revision":
			v, _ := e.str(a.X)
			d.Requirement = Requirement{Kind: l, Values: []string{v}}
		default:
			e.unknown(c, a)
		}
	}
	if d.Path == "" && d.URL == "" {
		e.errorAt(c.node, "a package dependency needs a path or a url")
		return d, false
	}
	return d, true
}

var targetKinds = map[string]string{
	"target": TargetRegular, "executableTarget": TargetExecutable, "testTarget": TargetTest,
	"systemLibrary": TargetSystem, "binaryTarget": TargetBinary, "plugin": TargetPlugin,
	"macro": TargetMacro,
}

var settingTools = map[string]string{
	"cSettings": "c", "cxxSettings": "cxx", "swiftSettings": "swift", "linkerSettings": "linker",
}

// target reads one of PackageDescription's target constructors.
func (e *evaluator) target(x ast.Expr) (Target, bool) {
	c, ok := e.call(x)
	kind, known := targetKinds[c.name]
	if !ok || !known {
		e.errorAt(x, "expected a target, as in '.target(name: \"Core\")'")
		return Target{}, false
	}
	t := Target{Kind: kind}
	for _, a := range c.args {
		l := e.label(a)
		switch l {
		case "name":
			t.Name, _ = e.str(a.X)
		case "dependencies":
			items, _ := e.list(a.X)
			for _, it := range items {
				if d, ok := e.targetDependency(it); ok {
					t.Dependencies = append(t.Dependencies, d)
				}
			}
		case "path":
			t.Path, _ = e.optionalStr(a.X)
		case "exclude":
			t.Exclude, _ = e.strs(a.X)
		case "sources":
			if !e.isNil(a.X) {
				t.Sources, _ = e.strs(a.X)
				t.HasSources = true
			}
		case "publicHeadersPath":
			t.PublicHeadersPath, _ = e.optionalStr(a.X)
		case "pkgConfig":
			t.PkgConfig, _ = e.optionalStr(a.X)
		case "url":
			t.URL, _ = e.str(a.X)
		case "checksum":
			t.Checksum, _ = e.str(a.X)
		case "packageAccess", "resources", "plugins", "providers":
			// Read and set aside: nothing vsc builds yet depends on them.
		case "cSettings", "cxxSettings", "swiftSettings", "linkerSettings":
			if e.isNil(a.X) {
				continue
			}
			items, _ := e.list(a.X)
			for _, it := range items {
				if s, ok := e.setting(settingTools[l], it); ok {
					t.Settings = append(t.Settings, s)
				}
			}
		default:
			e.unknown(c, a)
		}
	}
	if t.Name == "" {
		e.errorAt(c.node, "a target needs a name")
		return t, false
	}
	return t, true
}

// targetDependency reads `"Name"`, `.target(name:)`, `.product(name:package:)`
// and `.byName(name:)`, each with an optional condition.
func (e *evaluator) targetDependency(x ast.Expr) (TargetDependency, bool) {
	if _, isString := e.resolve(x).(*ast.StringLit); isString {
		s, ok := e.str(x)
		return TargetDependency{Kind: "byName", Name: s}, ok
	}
	c, ok := e.call(x)
	if !ok || (c.name != "target" && c.name != "product" && c.name != "byName") {
		e.errorAt(x, "expected a target dependency: a name, '.target(name:)' or '.product(name:package:)'")
		return TargetDependency{}, false
	}
	d := TargetDependency{Kind: c.name}
	for _, a := range c.args {
		switch e.label(a) {
		case "name":
			d.Name, _ = e.str(a.X)
		case "package":
			if !e.isNil(a.X) {
				d.Package, _ = e.str(a.X)
			}
		case "condition":
			d.Condition = e.condition(a.X)
		case "moduleAliases":
		default:
			e.unknown(c, a)
		}
	}
	return d, true
}

// setting reads one build setting: the case, its unlabelled values, and a
// trailing condition.
func (e *evaluator) setting(tool string, x ast.Expr) (Setting, bool) {
	c, ok := e.call(x)
	if !ok {
		e.errorAt(x, "expected a build setting, as in '.define(\"NAME\")'")
		return Setting{}, false
	}
	s := Setting{Tool: tool, Kind: c.name}
	var positional []ast.Expr
	var to *string
	for _, a := range c.args {
		switch e.label(a) {
		case "":
			positional = append(positional, a.X)
		case "to":
			to, _ = e.optionalStr(a.X)
		default:
			e.unknown(c, a)
		}
	}
	// A trailing unlabelled `.when(…)`, or nil, is the condition.
	if n := len(positional); n > 0 {
		last := positional[n-1]
		if cc, ok := e.call(last); ok && cc.name == "when" {
			s.Condition = e.condition(last)
			positional = positional[:n-1]
		} else if n > 1 && e.isNil(last) {
			positional = positional[:n-1]
		}
	}
	if len(positional) != 1 {
		e.errorAt(c.node, "'"+c.name+"' takes one value")
		return s, false
	}
	v := positional[0]
	switch c.name {
	case "define":
		name, ok := e.str(v)
		if !ok {
			return s, false
		}
		if tool != "swift" && to != nil {
			name += "=" + *to
		}
		s.Values = []string{name}
	case "unsafeFlags":
		flags, ok := e.strs(v)
		if !ok {
			return s, false
		}
		s.Values = flags
	case "interoperabilityMode", "swiftLanguageMode":
		mode, ok := e.caseName(v)
		if !ok {
			return s, false
		}
		s.Values = []string{mode}
	case "headerSearchPath", "linkedLibrary", "linkedFramework",
		"enableUpcomingFeature", "enableExperimentalFeature", "strictMemorySafety":
		str, ok := e.str(v)
		if !ok {
			return s, false
		}
		s.Values = []string{str}
	default:
		e.errorAt(c.node, "'"+c.name+"' is not a "+tool+" setting vsc reads")
		return s, false
	}
	return s, true
}

// condition reads `.when(platforms: [.macOS], configuration: .debug)`.
func (e *evaluator) condition(x ast.Expr) *Condition {
	if e.isNil(x) {
		return nil
	}
	c, ok := e.call(x)
	if !ok || c.name != "when" {
		e.errorAt(x, "expected a condition, as in '.when(platforms: [.macOS])'")
		return nil
	}
	cond := &Condition{}
	for _, a := range c.args {
		switch e.label(a) {
		case "platforms":
			items, _ := e.list(a.X)
			for _, it := range items {
				if name, ok := e.caseName(it); ok {
					if p, known := platformNames[name]; known {
						cond.Platforms = append(cond.Platforms, p)
					} else {
						e.errorAt(it, "'"+name+"' is not a platform vsc knows")
					}
				}
			}
		case "configuration":
			cond.Config, _ = e.caseName(a.X)
		case "traits":
		default:
			e.unknown(c, a)
		}
	}
	return cond
}
