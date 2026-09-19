package vsc

import (
	"errors"
	"fmt"
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/iface"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/derive"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/internal/sil/gen"
	"github.com/vertex-language/vsc/internal/sil/pass"
	"github.com/vertex-language/vsc/lower"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// A Diagnostic pairs a token.Diagnostic with the source file it originated from.
// File is nil for module-level failures that lack a specific source location.
type Diagnostic struct {
	token.Diagnostic
	File *token.File
}

func (d Diagnostic) String() string {
	if d.File == nil {
		return d.Message
	}
	return d.Print(d.File)
}

// Errors reports whether any of these stop a compilation.
func Errors(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == token.Error {
			return true
		}
	}
	return false
}

// A Source is one file to compile.
type Source struct {
	Name string
	Text []byte
}

// Options configures the compilation process.
type Options struct {
	// Module is the name the module is compiled as.
	Module string
	// Target is the machine architecture to lower for.
	Target ir.Target
	// Stop specifies which phase to stop after (zero value runs all phases).
	Stop Phase
	// PackagePaths are search roots for string import paths (e.g. `import "std/fmt"`).
	PackagePaths []string
	// ImportPaths are search directories for interface files (e.g. `import Lib`).
	ImportPaths []string
	// Packages resolves a string import path that no search root holds.
	// It is what makes `import "net/tcp"` work in a program that has
	// never seen the package: see vsc/importer, which the command line
	// supplies. Nil never fetches anything, which is what compiling
	// against what is already on disk wants.
	Packages PackageResolver
}

// A PackageResolver finds the source of a package named by a string
// import path but not present as a folder.
type PackageResolver interface {
	// Local is the directory of source for path, written in a file in
	// fromDir, found without fetching anything: a checkout the build was
	// told to use instead, or the checkout fromDir is itself inside, where
	// path is that checkout or a folder of it. "" where neither answers.
	Local(path, fromDir string) (string, error)
	// Fetch is the directory of source for path, downloading the package
	// it is in if need be, or "" where the path is not one this resolver
	// can fetch.
	Fetch(path string) (string, error)
}

// A TargetBuilder is a PackageResolver that can also build what a folder
// of source needs beside it before the folder is read: the C-family
// targets its package's manifest has it depend on, whose interfaces the
// folder imports by module name -- `import cwindow` in a target that
// depends on a C target called cwindow. Without one, such a folder imports
// only what the import paths already hold.
type TargetBuilder interface {
	// TargetModules builds the modules the target in dir depends on and is
	// the directory their interfaces are in, or "" where dir is no target
	// of a package or depends on nothing to build.
	TargetModules(dir string) (string, error)
}

// A Phase is one step of the compiler pipeline, in execution order.
type Phase int

const (
	// All runs every phase through code generation.
	All Phase = iota
	// Parsed stops after syntax tree construction.
	Parsed
	// Checked stops after type checking and name resolution.
	Checked
	// Raw stops after raw SIL generation.
	Raw
	// Canonical stops after mandatory SIL passes.
	Canonical
	// Lowered stops after ownership lowering.
	Lowered
)

// A Unit contains the artifacts produced across compilation phases.
type Unit struct {
	// Files are the parsed syntax trees.
	Files []*ast.File
	// Positions maps each file to its token position table.
	Positions []*token.File
	// Info holds semantic analysis results.
	Info *analyzer.Info
	// SIL is the ownership IR.
	SIL *sil.Module
	// VIR is the machine IR.
	VIR *ir.Module
	// Packages are imported source packages in topological dependency order.
	Packages []Package

	// derived is how many of Files the compiler wrote itself; see derive.
	derived int
}

// A Package represents an imported source folder and its target module name.
type Package struct {
	// Name is the module name used for mangling symbols.
	Name string
	// Dir is the directory where the source files were found.
	Dir string
	// Sources are the package's source files.
	Sources []Source
	// ImportPaths are where the modules built for it are, which compiling
	// it needs beside the program's own; see TargetBuilder.
	ImportPaths []string
}

// Compile runs the compiler phases in order, stopping at the first phase that reports
// an error. A non-nil Unit is returned with partial results up to the stopping point.
func Compile(srcs []Source, opts Options) (*Unit, []Diagnostic) {
	u := &Unit{}
	var diags []Diagnostic

	for _, src := range srcs {
		tf := token.NewFile(src.Name, src.Text)
		file, ds := parser.ParseFile(tf, 0)
		u.Files = append(u.Files, file)
		u.Positions = append(u.Positions, tf)
		diags = append(diags, attribute(ds, tf)...)
	}
	if opts.Stop == Parsed || Errors(diags) {
		return u, diags
	}
	if file, unit := derived(u.Files); file != nil {
		u.Files = append(u.Files, file)
		u.Positions = append(u.Positions, unit)
		u.derived++
	}

	imports, pkgs, importDiags := loadImports(u.Files, u.Positions, opts.ImportPaths, opts.PackagePaths, opts.Packages)
	u.Packages = pkgs
	diags = append(diags, importDiags...)
	if Errors(diags) {
		return u, diags
	}
	info, checks := analyzer.CheckModule(opts.Module, u.Files, imports)
	u.Info = info
	diags = append(diags, attribute(checks, u.only())...)
	if opts.Stop == Checked || Errors(diags) {
		return u, diags
	}

	m, gens := gen.Files(opts.Module, u.Files, info)
	u.SIL = m
	diags = append(diags, attribute(gens, u.only())...)
	if opts.Stop == Raw || Errors(diags) {
		return u, diags
	}

	if err := pass.Mandatory(m); err != nil {
		return u, append(diags, phaseError(err))
	}
	if opts.Stop == Canonical {
		return u, diags
	}

	if err := pass.LowerOwnership(m); err != nil {
		return u, append(diags, phaseError(err))
	}
	if opts.Stop == Lowered {
		return u, diags
	}

	if !opts.Target.Valid() {
		return u, append(diags, phaseError(errNoTarget))
	}
	out, err := lower.Module(m, opts.Target, lower.Options{
		SymbolPrefix: SymbolPrefix(opts.Target),
	})
	if err != nil {
		return u, append(diags, phaseError(err))
	}
	u.VIR = out
	return u, diags
}

// only returns the single source file if there is exactly one, or nil if multiple.
func (u *Unit) only() *token.File {
	if len(u.Positions)-u.derived == 1 {
		return u.Positions[0]
	}
	return nil
}

// derived is the file of declarations Swift derives for the types in
// files -- a struct's `==` where it says it is Equatable and writes none
// -- parsed, or nil where there are none.
func derived(files []*ast.File) (*ast.File, *token.File) {
	text := derive.Source(files)
	if text == nil {
		return nil, nil
	}
	tf := token.NewFile("<derived conformances>", text)
	file, ds := parser.ParseFile(tf, 0)
	if len(ds) > 0 {
		return nil, nil
	}
	return file, tf
}

// attribute pairs each diagnostic with the file it is about: the one it
// names, where it names one -- a module of several files is lowered file
// by file, and each diagnostic knows which -- or else f.
func attribute(ds []token.Diagnostic, f *token.File) []Diagnostic {
	out := make([]Diagnostic, len(ds))
	for i, d := range ds {
		file := f
		if d.File != nil {
			file = d.File
		}
		out[i] = Diagnostic{Diagnostic: d, File: file}
	}
	return out
}

// phaseError converts an unpositioned error into a Diagnostic.
func phaseError(err error) Diagnostic {
	return Diagnostic{Diagnostic: token.Diagnostic{
		Pos:      token.NoPos,
		End:      token.NoPos,
		Severity: token.Error,
		Message:  err.Error(),
	}}
}

// errNoTarget is reported when lowering is requested without a valid target.
var errNoTarget = errors.New("no target: lowering needs a machine to lower for")

// loadImports resolves and loads all transitive module and package imports.
func loadImports(files []*ast.File, units []*token.File, paths, pkgPaths []string,
	packages PackageResolver) ([]analyzer.Import, []Package, []Diagnostic) {
	l := &importer{paths: paths, pkgPaths: pkgPaths, packages: packages, seen: map[string]bool{}}
	for i, f := range files {
		unit := f.Unit
		if i < len(units) && units[i] != nil {
			unit = units[i]
		}
		l.readAll(f, unit, "")
	}
	return l.out, l.pkgs, l.diags
}

// An importer resolves and loads imported modules and packages.
type importer struct {
	paths    []string
	pkgPaths []string
	packages PackageResolver
	fetched  map[string]string
	fetchErr map[string]error
	seen     map[string]bool
	out      []analyzer.Import
	pkgs     []Package
	diags    []Diagnostic
}

// readAll reads every module a file imports. `via` is the module
// whose interface asked for them, empty for the program's own.
func (l *importer) readAll(f *ast.File, unit *token.File, via string) {
	for _, stmt := range f.Stmts {
		decl, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		imp, ok := decl.D.(*ast.ImportDecl)
		if !ok || unit == nil {
			continue
		}
		// The string form names a folder of source rather than a
		// module built elsewhere. A group is punctuation standing for
		// one import each, so it is the same loop.
		for _, spec := range imp.Paths {
			l.readFolder(spec, imp, unit, via)
		}
		if len(imp.Path) == 0 {
			continue
		}
		// The first component names the module; a dotted path beyond
		// it names something inside one, which this does not narrow
		// to yet.
		name := imp.Path[0].Text(unit)
		if name == "" || l.seen[name] || builtinModule(name) {
			continue
		}
		l.seen[name] = true
		l.read(name, imp, unit, via)
	}
}

// builtinModule reports whether name is a built-in module provided implicitly.
func builtinModule(name string) bool {
	switch name {
	case "Swift", "Builtin", "_Concurrency", "_StringProcessing",
		"_SwiftConcurrencyShims", "_math":
		return true
	}
	return false
}

// read loads a single module interface and its transitive imports.
func (l *importer) read(name string, at *ast.ImportDecl, unit *token.File, via string) {
	severity := token.Error
	if via != "" {
		severity = token.Warn
	}
	fail := func(msg string) {
		if via != "" {
			msg += " (imported by " + via + ")"
		}
		l.diags = append(l.diags, Diagnostic{File: unit, Diagnostic: token.Diagnostic{
			Pos:      at.Pos(),
			End:      at.End(),
			Severity: severity,
			File:     unit,
			Message:  msg,
		}})
	}

	path, found := findInterface(name, l.paths)
	if !found {
		fail("no such module '" + name + "': looked for " + name + iface.Extension +
			", " + name + ".swiftinterface, and " + name + ".swiftmodule/")
		return
	}
	text, err := os.ReadFile(path)
	if err != nil {
		fail("cannot read '" + name + "': " + err.Error())
		return
	}
	if h := iface.ReadHeader(text); h.Resilient {
		fail("'" + name + "' was built with -enable-library-evolution, which is a " +
			"different ABI: its types have no layout a client may rely on. Build it " +
			"without that flag, or mark the types this program uses @frozen")
		return
	}
	tf := token.NewFile(path, text)
	parsed, ds := parser.ParseFile(tf, 0)
	if len(ds) > 0 {
		fail("'" + name + "' has an interface this compiler cannot read: " + path)
		return
	}
	l.readAll(parsed, tf, name)
	l.out = append(l.out, analyzer.Import{
		Name:  name,
		Files: []*ast.File{parsed},
		Units: []*token.File{tf},
	})
}

// findInterface searches paths for a module interface (.vinterface, .swiftinterface, or .swiftmodule).
func findInterface(name string, paths []string) (string, bool) {
	for _, dir := range paths {
		for _, ext := range []string{iface.Extension, ".swiftinterface"} {
			candidate := filepath.Join(dir, name+ext)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, true
			}
		}
		if found, ok := inSwiftModule(filepath.Join(dir, name+".swiftmodule")); ok {
			return found, true
		}
	}
	return "", false
}

// inSwiftModule locates the architecture-specific swiftinterface file inside dir.
func inSwiftModule(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".swiftinterface") ||
			strings.HasSuffix(n, ".private.swiftinterface") {
			continue
		}
		if strings.HasPrefix(n, arch) {
			return filepath.Join(dir, n), true
		}
	}
	return "", false
}

// importPathText extracts the unescaped path string from a single-segment string literal.
func importPathText(lit *ast.StringLit, unit *token.File) (string, bool) {
	if lit == nil || lit.Multiline || len(lit.Segments) != 1 {
		return "", false
	}
	text, ok := lit.Segments[0].(*ast.StringText)
	if !ok {
		return "", false
	}
	return string(unit.Slice(text.Lo, text.Hi)), true
}

// findFolder resolves an import path to a directory, checking relative paths or package roots.
func findFolder(path, fromDir string, pkgPaths []string) (string, bool) {
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		dir := filepath.Join(fromDir, path)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, true
		}
		return "", false
	}
	for _, root := range pkgPaths {
		dir := filepath.Join(root, filepath.FromSlash(path))
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, true
		}
	}
	return "", false
}

// folder is the directory an import path names, found in this order:
//
//  1. A relative path is a folder beside the file, and nothing else.
//  2. The resolver's local answer: a checkout named with -replace, or the
//     checkout the importing file is in, where the path is that checkout
//     or a folder of it. A package's tests build against the package on
//     disk, and one folder of a repository imports another at the same
//     revision.
//  3. A folder under a search root (-P, VERTEXPATH).
//  4. The resolver's fetch: the standard library from
//     github.com/vertex-language, or a repository the path names.
func (l *importer) folder(path, fromDir string) (string, error) {
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		if dir, found := findFolder(path, fromDir, nil); found {
			return dir, nil
		}
		return "", fmt.Errorf("no such package '%s': looked in %s", path, fromDir)
	}
	if l.packages != nil {
		dir, err := l.packages.Local(path, fromDir)
		if err != nil {
			return "", err
		}
		if dir != "" {
			return dir, nil
		}
	}
	if dir, found := findFolder(path, fromDir, l.pkgPaths); found {
		return dir, nil
	}
	dir, err := l.fetchFolder(path)
	if err != nil {
		return "", err
	}
	if dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("no such package '%s': looked in the search roots", path)
}

// fetchFolder resolves an import path through the caller's resolver. It
// is "" where there is none, or where the resolver does not handle the
// path, which leaves the ordinary "no such package" for the caller.
//
// A path is resolved once per compile however many files import it.
func (l *importer) fetchFolder(path string) (string, error) {
	if l.packages == nil || strings.HasPrefix(path, ".") {
		return "", nil
	}
	if dir, ok := l.fetched[path]; ok {
		return dir, nil
	}
	if err, ok := l.fetchErr[path]; ok {
		return "", err
	}
	dir, err := l.packages.Fetch(path)
	if err != nil {
		if l.fetchErr == nil {
			l.fetchErr = map[string]error{}
		}
		l.fetchErr[path] = err
		return "", err
	}
	if l.fetched == nil {
		l.fetched = map[string]string{}
	}
	l.fetched[path] = dir
	return dir, nil
}

// packageNameOf inspects parsed files for a package declaration, returning "" if absent.
func packageNameOf(files []*ast.File, units []*token.File) string {
	for i, f := range files {
		unit := f.Unit
		if i < len(units) && units[i] != nil {
			unit = units[i]
		}
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			if pkg, ok := decl.D.(*ast.PackageDecl); ok && pkg.Name != nil && unit != nil {
				return pkg.Name.Text(unit)
			}
		}
	}
	return ""
}

// SourceExtension is the file extension for Vertex source files (.vs).
const SourceExtension = ".vs"

// readFolder loads a package from a directory of source files.
func (l *importer) readFolder(spec *ast.ImportPath, at *ast.ImportDecl, unit *token.File, via string) {
	severity := token.Error
	if via != "" {
		severity = token.Warn
	}
	fail := func(msg string) {
		if via != "" {
			msg += " (imported by " + via + ")"
		}
		l.diags = append(l.diags, Diagnostic{File: unit, Diagnostic: token.Diagnostic{
			Pos:      spec.Pos(),
			End:      spec.End(),
			Severity: severity,
			File:     unit,
			Message:  msg,
		}})
	}

	path, ok := importPathText(spec.Path, unit)
	if !ok || path == "" {
		fail("an import path must be a plain string")
		return
	}
	dir, err := l.folder(path, filepath.Dir(unit.Name()))
	if err != nil {
		fail(err.Error())
		return
	}
	sources, err := filepath.Glob(filepath.Join(dir, "*"+SourceExtension))
	if err != nil || len(sources) == 0 {
		fail("package '" + path + "' has no " + SourceExtension + " files: " + dir)
		return
	}
	sort.Strings(sources)

	var files []*ast.File
	var units []*token.File
	var srcs []Source
	for _, src := range sources {
		text, err := os.ReadFile(src)
		if err != nil {
			fail("cannot read '" + src + "': " + err.Error())
			return
		}
		tf := token.NewFile(src, text)
		parsed, ds := parser.ParseFile(tf, 0)
		if len(ds) > 0 {
			fail("package '" + path + "' has a file this compiler cannot read: " + src)
			return
		}
		files = append(files, parsed)
		units = append(units, tf)
		srcs = append(srcs, Source{Name: src, Text: text})
	}

	if file, tf := derived(files); file != nil {
		files = append(files, file)
		units = append(units, tf)
	}

	name := packageNameOf(files, units)
	if name == "" {
		name = lastSegment(path)
	}
	if name == "" {
		fail("cannot name the module for '" + path + "': add a package declaration")
		return
	}
	as := name
	if spec.Alias != nil {
		as = spec.Alias.Text(unit)
	}
	if l.seen[as] {
		return
	}
	l.seen[as] = true

	// What the folder's package builds for it comes first, so that its
	// imports of those modules are found, here and when it is compiled.
	var paths []string
	if tb, ok := l.packages.(TargetBuilder); ok {
		modules, err := tb.TargetModules(dir)
		if err != nil {
			fail("package '" + path + "': " + err.Error())
			return
		}
		if modules != "" {
			paths = append(paths, modules)
			if !slices.Contains(l.paths, modules) {
				l.paths = append(l.paths, modules)
			}
		}
	}

	for i, f := range files {
		l.readAll(f, units[i], name)
	}
	l.out = append(l.out, analyzer.Import{Name: name, As: as, Files: files, Units: units})
	l.pkgs = append(l.pkgs, Package{Name: name, Dir: dir, Sources: srcs, ImportPaths: paths})
}

// lastSegment returns the trailing folder name from a path.
func lastSegment(path string) string {
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
