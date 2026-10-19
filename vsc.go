package vsc

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/iface"
	"github.com/vertex-language/vsc/timing"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/ifconfig"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/internal/sil/gen"
	"github.com/vertex-language/vsc/internal/sil/pass"
	"github.com/vertex-language/vsc/lower"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/pkg"
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
	// SkipVerify runs SIL passes without the verifier for debugging (--emit sil or vir with -skip-verify).
	SkipVerify bool
	// PackagePaths are search roots for string import paths (e.g. `import "std/fmt"`).
	PackagePaths []string
	// ImportPaths are search directories for interface files (e.g. `import Lib`).
	ImportPaths []string
	// Packages resolves external string import paths not found in search roots.
	// When nil, resolution does not fetch from remote repositories.
	Packages PackageResolver
	// AfterImports, when set, is called after imports are loaded. Returning
	// false halts compilation early with Unit.Packages populated.
	AfterImports func(pkgs []Package, opaque bool) bool
	// OnPackage is invoked as soon as each imported package is ready,
	// allowing concurrent compilation while remaining imports are read.
	OnPackage func(Package)
	// Summaries caches pruned interface source for imported folders to speed
	// up parsing. When nil, full sources are parsed on every import.
	Summaries SummaryCache
}

// A SummaryCache caches pruned source packages keyed by content and target architecture.
type SummaryCache interface {
	Get(key [32]byte) ([]byte, bool)
	Put(key [32]byte, data []byte)
}

// A PackageResolver finds the source of a package named by an import path.
type PackageResolver interface {
	// Local returns the local checkout directory for path without network access,
	// or "" if not found locally.
	Local(path, fromDir string) (string, error)
	// Fetch downloads and returns the directory for path, or "" if unhandled.
	Fetch(path string) (string, error)
}

// A NativeBinder provides Vertex bindings for C++ package modules.
type NativeBinder interface {
	// NativeSources returns generated Vertex declarations exported by the C++
	// module in dir.
	NativeSources(dir, path, pkgName string, public bool) ([]Source, error)
}

// A NativePrefetcher begins parsing C++ headers asynchronously before
// bindings are explicitly requested.
type NativePrefetcher interface {
	PrefetchNative(dir string, sources []string)
	// PrefetchBindings begins binding the module as pkgName once source is read.
	PrefetchBindings(dir, pkgName string, public bool)
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

	// derived is the count of synthesized files for derived conformances.
	derived int
}

// A Package represents an imported source folder and its target module name.
type Package struct {
	// Name is the module name used for mangling symbols.
	Name string
	// Dir is the directory where the source files were found.
	Dir string
	// Sources are the package's .vs files and C++ export declarations.
	Sources []Source
	// Native indicates the package contains a C++ module.
	Native bool
	// Deps are the dependency directories in topological order.
	Deps []string
	// Opaque indicates the package imports an interface file not tracked in Deps.
	Opaque bool
	// Surface is a content hash of the package's public interface for early-cutoff caching.
	Surface [32]byte
}

// Compile runs the compiler phases in order, stopping at the first phase that reports
// an error. A non-nil Unit is returned with partial results up to the stopping point.
func Compile(srcs []Source, opts Options) (*Unit, []Diagnostic) {
	u := &Unit{}
	var diags []Diagnostic

	ifcfg := ifconfig.ForTarget(opts.Target.String())
	done := timing.Start("parse")
	for _, src := range srcs {
		tf := token.NewFile(src.Name, src.Text)
		file, ds := parser.ParseFile(tf, 0)
		// Resolve #if for the target before semantic analysis.
		ifconfig.Resolve(file, ifcfg)
		u.Files = append(u.Files, file)
		u.Positions = append(u.Positions, tf)
		diags = append(diags, attribute(ds, tf)...)
	}
	// Compile and append stylesheet sources for the program's folders.
	styles, styleDiags := programStyles(u.Files, u.Positions, srcs, opts)
	diags = append(diags, styleDiags...)
	for _, src := range styles {
		tf := token.NewFile(src.Name, src.Text)
		file, ds := parser.ParseFile(tf, 0)
		u.Files = append(u.Files, file)
		u.Positions = append(u.Positions, tf)
		diags = append(diags, attribute(ds, tf)...)
	}
	done()
	if opts.Stop == Parsed || Errors(diags) {
		return u, diags
	}
	done = timing.Start("imports")
	imports, pkgs, importDiags := loadImports(u.Files, u.Positions, opts.ImportPaths, opts.PackagePaths, opts.Packages, ifcfg, opts.Target, opts.Summaries, opts.OnPackage)
	done()
	u.Packages = pkgs
	diags = append(diags, importDiags...)
	if Errors(diags) {
		return u, diags
	}
	if opts.AfterImports != nil {
		opaque := len(imports) > len(pkgs)
		for _, p := range pkgs {
			opaque = opaque || p.Opaque
		}
		if !opts.AfterImports(pkgs, opaque) {
			return u, diags
		}
	}
	done = timing.Start("check")
	info, checks := analyzer.CheckModule(opts.Module, u.Files, imports)
	done()
	u.Info = info
	// Append synthesized declarations for derived conformances.
	if info.Derived != nil {
		u.Files = append(u.Files, info.Derived)
		u.Positions = append(u.Positions, info.Derived.Unit)
		u.derived++
	}
	diags = append(diags, attribute(checks, u.only())...)
	if opts.Stop == Checked || Errors(diags) {
		return u, diags
	}

	done = timing.Start("silgen")
	m, gens := gen.Files(opts.Module, u.Files, info)
	done()
	u.SIL = m
	diags = append(diags, attribute(gens, u.only())...)
	if opts.Stop == Raw || Errors(diags) {
		return u, diags
	}

	done = timing.Start("sil passes")
	defer func() { done() }()
	mandatory := pass.Mandatory
	if opts.SkipVerify {
		mandatory = pass.MandatoryUnverified
	}
	if err := mandatory(m); err != nil {
		return u, append(diags, phaseError(err))
	}
	if opts.Stop == Canonical {
		return u, diags
	}
	if err := pass.Optimize(m); err != nil {
		return u, append(diags, phaseError(err))
	}

	if err := pass.LowerOwnership(m); err != nil {
		return u, append(diags, phaseError(err))
	}
	if opts.Stop == Lowered {
		return u, diags
	}

	done()
	done = func() {}
	if !opts.Target.Valid() {
		return u, append(diags, phaseError(errNoTarget))
	}
	// Each kernel, for the devices: what its descriptor holds.
	kernels, kdiags := compileKernels(m, opts.Target)
	diags = append(diags, kdiags...)
	if Errors(diags) {
		return u, diags
	}
	doneVIR := timing.Start("sil to vir")
	out, err := lower.Module(m, opts.Target, lower.Options{
		SymbolPrefix: SymbolPrefix(opts.Target),
		Kernels:      kernels,
	})
	doneVIR()
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

// attribute pairs each diagnostic with the file it originated from.
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
	packages PackageResolver, ifcfg ifconfig.Config, target ir.Target, summaries SummaryCache, onPackage func(Package)) ([]analyzer.Import, []Package, []Diagnostic) {
	l := &importer{paths: paths, pkgPaths: pkgPaths, packages: packages, seen: map[string]bool{}, direct: map[string]bool{}, ifcfg: ifcfg, target: target, summaries: summaries, onPackage: onPackage}
	for i, f := range files {
		unit := f.Unit
		if i < len(units) && units[i] != nil {
			unit = units[i]
		}
		l.readAll(f, unit, "")
	}
	// Mark direct imports so local imports take precedence over transitive ones.
	for i := range l.out {
		l.out[i].Direct = l.direct[l.out[i].Name] || (l.out[i].As != "" && l.direct[l.out[i].As])
	}
	return l.out, l.pkgs, l.diags
}

// An importer resolves and loads imported modules and packages.
type importer struct {
	// ifcfg settles `#if` blocks of imported sources for the target.
	ifcfg ifconfig.Config
	// target determines platform and architecture filtering for files.
	target   ir.Target
	paths    []string
	pkgPaths []string
	packages PackageResolver
	fetched  map[string]string
	fetchErr map[string]error
	seen     map[string]bool
	// direct tracks modules directly imported by the program's files.
	direct map[string]bool
	// names maps package directory paths to package names.
	names map[string]string
	// modules maps package directory paths to mangled module names.
	modules map[string]string
	// loading tracks in-flight folders; loadedAt indexes loaded imports in out.
	loading  map[string]bool
	loadedAt map[string]int
	// ahead tracks packages being prefetched and parsed in parallel.
	ahead map[string]*prefetch
	// found caches resolved package directories by import path and importing directory.
	found map[[2]string]foundFolder
	// summaries caches pruned package sources.
	summaries SummaryCache
	// onPackage is Options.OnPackage.
	onPackage func(Package)
	out       []analyzer.Import
	pkgs      []Package
	diags     []Diagnostic
}

// readAll reads every module a file imports. `via` is the module
// whose interface asked for them, empty for the program's own.
// It returns imported folder paths and whether an opaque interface file was imported.
func (l *importer) readAll(f *ast.File, unit *token.File, via string) (deps []string, opaque bool) {
	for _, stmt := range f.Stmts {
		decl, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		imp, ok := decl.D.(*ast.ImportDecl)
		if !ok || unit == nil {
			continue
		}
		for _, spec := range imp.Paths {
			if dir := l.readFolder(spec, imp, unit, via); dir != "" {
				deps = append(deps, dir)
			}
		}
		if len(imp.Path) == 0 {
			continue
		}
		// The first component names the module; a dotted path beyond
		// it names something inside one, which this does not narrow
		// to yet.
		name := imp.Path[0].Text(unit)
		if name == "" || builtinModule(name) {
			continue
		}
		opaque = true
		if l.seen[name] {
			continue
		}
		l.seen[name] = true
		l.read(name, imp, unit, via)
	}
	return deps, opaque
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
	ifconfig.Resolve(parsed, l.ifcfg)
	l.readAll(parsed, tf, name)
	l.out = append(l.out, analyzer.Import{
		Name:  name,
		Files: []*ast.File{parsed},
		Units: []*token.File{tf},
	})
}

// moduleDeps returns the unique names of modules imported by the package files.
func (l *importer) moduleDeps(files []*ast.File, units []*token.File, dir string) []string {
	var out []string
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
			imp, ok := decl.D.(*ast.ImportDecl)
			if !ok || unit == nil {
				continue
			}
			for _, spec := range imp.Paths {
				path, ok := importPathText(spec.Path, unit)
				if !ok {
					continue
				}
				d, err := l.folder(path, dir)
				if err != nil {
					continue
				}
				if name := l.modules[d]; name != "" && !slices.Contains(out, name) {
					out = append(out, name)
				}
			}
			if len(imp.Path) > 0 {
				if name := imp.Path[0].Text(unit); name != "" && !slices.Contains(out, name) {
					out = append(out, name)
				}
			}
		}
	}
	return out
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
	k := [2]string{path, fromDir}
	if r, ok := l.found[k]; ok {
		return r.dir, r.err
	}
	dir, err := l.resolveFolder(path, fromDir)
	if l.found == nil {
		l.found = map[[2]string]foundFolder{}
	}
	l.found[k] = foundFolder{dir, err}
	return dir, err
}

type foundFolder struct {
	dir string
	err error
}

// resolveFolder is folder, unremembered.
func (l *importer) resolveFolder(path, fromDir string) (string, error) {
	// The built-in gpu module is the compiler's own, whatever is on disk.
	if path == GPUModule {
		return GPUSourceDir()
	}
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

// MarkupExtension is the file extension for Vertex with markup (.vsx),
// which is package source beside .vs files. See proposed_vsx.md.
const MarkupExtension = ".vsx"

// IsSourceFile reports whether a file name is Vertex source: .vs or .vsx.
func IsSourceFile(name string) bool {
	ext := filepath.Ext(name)
	return ext == SourceExtension || ext == MarkupExtension
}

// SourceFiles is dir's Vertex source files, .vs and .vsx, sorted.
func SourceFiles(dir string) []string {
	vs, _ := filepath.Glob(filepath.Join(dir, "*"+SourceExtension))
	vsx, _ := filepath.Glob(filepath.Join(dir, "*"+MarkupExtension))
	all := append(vs, vsx...)
	sort.Strings(all)
	return all
}

// readFolder loads a package from a directory of source files.
func (l *importer) readFolder(spec *ast.ImportPath, at *ast.ImportDecl, unit *token.File, via string) (dir string) {
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
		return ""
	}
	dir, err := l.folder(path, filepath.Dir(unit.Name()))
	if err != nil {
		fail(err.Error())
		return ""
	}
	// Fast-path: return immediately if this package directory is already loaded.
	if name, ok := l.names[dir]; ok {
		as := name
		if spec.Alias != nil {
			as = spec.Alias.Text(unit)
		}
		if via == "" {
			l.direct[as] = true
		}
		if i, loaded := l.loadedAt[dir]; loaded {
			l.bind(i, as)
			return dir
		}
		if l.loading[dir] {
			return dir
		}
	}
	pre := l.ahead[dir]
	if pre == nil {
		pre = l.readAhead(dir)
	}
	<-pre.done
	// Remove from ahead map; ASTs are unique per import because the checker mutates them.
	delete(l.ahead, dir)
	folder, err := pre.folder, pre.err
	if err != nil {
		fail("package '" + path + "': " + err.Error())
		return ""
	}
	if folder.Empty() {
		fail("package '" + path + "' has no source for this target: " + dir)
		return ""
	}

	var files []*ast.File
	var units []*token.File
	var srcs []Source
	add := func(r readFile) bool {
		if r.bad {
			fail("package '" + path + "' has a file this compiler cannot read: " + r.src.Name)
			return false
		}
		files = append(files, r.parsed)
		units = append(units, r.unit)
		srcs = append(srcs, r.src)
		return true
	}
	read := pre.read
	for i, name := range folder.Vertex {
		if err := read[i].err; err != nil {
			fail("cannot read '" + name + "': " + err.Error())
			return ""
		}
		if !add(read[i]) {
			return ""
		}
	}

	name := packageNameOf(files, units)
	if name != "" {
		if l.names == nil {
			l.names = map[string]string{}
		}
		l.names[dir] = name
	}
	if name == "" {
		name = lastSegment(path)
	}
	if name == "" {
		fail("cannot name the module for '" + path + "': add a package declaration")
		return ""
	}
	as := name
	if spec.Alias != nil {
		as = spec.Alias.Text(unit)
	}
	if via == "" {
		l.direct[as] = true
	}
	if l.loading[dir] {
		return dir
	}
	if l.loading == nil {
		l.loading = map[string]bool{}
		l.loadedAt = map[string]int{}
	}
	l.loading[dir] = true

	// Compile and append stylesheet source if package contains .vss files.
	if len(folder.Styles) > 0 {
		gen, ds := styleSource(dir, packageNameOf(files, units), importPaths(files, units), l.folder)
		l.diags = append(l.diags, ds...)
		if gen != nil && !add(parseSource(*gen, l.ifcfg)) {
			return ""
		}
	}

	// Generate Vertex bridge declarations for C++ module exports.
	if len(folder.Native) > 0 {
		nb, ok := l.packages.(NativeBinder)
		if !ok {
			fail("package '" + path + "' has C++, which this build does not compile")
			return ""
		}
		native, err := nb.NativeSources(dir, path, name, len(folder.Vertex) == 0)
		if err != nil {
			fail("package '" + path + "': " + err.Error())
			return ""
		}
		for _, src := range native {
			if !add(parseSource(src, l.ifcfg)) {
				return ""
			}
		}
	}

	// Asynchronously prefetch transitive imports.
	for i, f := range files {
		l.prefetchImports(f, units[i])
	}
	var deps []string
	opaque := false
	for i, f := range files {
		d, o := l.readAll(f, units[i], name)
		opaque = opaque || o
		for _, x := range d {
			if !slices.Contains(deps, x) {
				deps = append(deps, x)
			}
		}
	}
	module := moduleNameOf(path, name)
	if l.modules == nil {
		l.modules = map[string]string{}
	}
	l.modules[dir] = module
	l.out = append(l.out, analyzer.Import{Name: module, As: as, Files: files, Units: units, Deps: l.moduleDeps(files, units, dir)})
	l.loadedAt[dir] = len(l.out) - 1
	l.pkgs = append(l.pkgs, Package{Name: module, Dir: dir, Sources: srcs, Native: len(folder.Native) > 0, Deps: deps, Opaque: opaque,
		Surface: surface(files, units)})
	if l.onPackage != nil {
		l.onPackage(l.pkgs[len(l.pkgs)-1])
	}
	return dir
}

// A prefetch is a folder read and parsed ahead of the walk: its files for
// this target, and each one's source and tree, in order.
type prefetch struct {
	done   chan struct{}
	folder *pkg.Folder
	err    error
	read   []readFile
}

// readAhead starts reading dir: which files it builds for the target, and
// each of them read and parsed, all at once.
func (l *importer) readAhead(dir string) *prefetch {
	pre := &prefetch{done: make(chan struct{})}
	if l.ahead == nil {
		l.ahead = map[string]*prefetch{}
	}
	l.ahead[dir] = pre
	use := l.target.Use()
	cfg := l.ifcfg
	packages := l.packages
	summaries := l.summaries
	target := l.target.String()
	pf, _ := packages.(NativePrefetcher)
	go func() {
		defer close(pre.done)
		if pf != nil {
			defer bindAhead(pf, dir, pre)
		}
		pre.folder, pre.err = pkg.ReadFolder(dir, pkg.PlatformOf(use), pkg.ArchOf(use))
		if pre.err != nil || pre.folder.Empty() {
			return
		}
		if pf, ok := packages.(NativePrefetcher); ok && len(pre.folder.Native) > 0 {
			pf.PrefetchNative(dir, pre.folder.Native)
		}
		names := pre.folder.Vertex
		pre.read = make([]readFile, len(names))
		texts := make([][]byte, len(names))
		each := func(fn func(i int)) {
			var wg sync.WaitGroup
			for i := range names {
				wg.Add(1)
				go func() {
					defer wg.Done()
					fn(i)
				}()
			}
			wg.Wait()
		}
		each(func(i int) {
			texts[i], pre.read[i].err = os.ReadFile(names[i])
		})
		for _, r := range pre.read {
			if r.err != nil {
				return
			}
		}
		// What was read of this folder before, pruned, where nothing in
		// it has changed since: the importer's trees are of that, and the
		// package keeps its source for its own compile.
		var key [32]byte
		if summaries != nil {
			key = summaryKey(target, names, texts)
			if data, ok := summaries.Get(key); ok {
				if pruned, ok := decodeSummary(data, len(names)); ok {
					each(func(i int) {
						r := parseSource(Source{Name: names[i], Text: pruned[i]}, cfg)
						r.src = Source{Name: names[i], Text: texts[i]}
						pre.read[i] = r
					})
					if !anyBad(pre.read) {
						return
					}
				}
			}
		}
		each(func(i int) {
			pre.read[i] = parseSource(Source{Name: names[i], Text: texts[i]}, cfg)
		})
		if summaries != nil && !anyBad(pre.read) {
			files := make([]*ast.File, len(names))
			units := make([]*token.File, len(names))
			for i, r := range pre.read {
				files[i], units[i] = r.parsed, r.unit
			}
			summaries.Put(key, encodeSummary(prune(files, units)))
		}
	}()
	return pre
}

// bindAhead prefetches C++ bindings for folder dir once its package name is known.
func bindAhead(pf NativePrefetcher, dir string, pre *prefetch) {
	if pre.err != nil || pre.folder == nil || len(pre.folder.Native) == 0 || anyBad(pre.read) {
		return
	}
	var files []*ast.File
	var units []*token.File
	for _, r := range pre.read {
		files, units = append(files, r.parsed), append(units, r.unit)
	}
	name := packageNameOf(files, units)
	if name == "" {
		name = filepath.Base(dir)
	}
	pf.PrefetchBindings(dir, name, len(pre.folder.Vertex) == 0)
}

func anyBad(rs []readFile) bool {
	for _, r := range rs {
		if r.bad || r.err != nil {
			return true
		}
	}
	return false
}

// summaryKey covers everything a folder's pruned text depends on: which
// files, what they say, and the target their #if blocks are settled for.
func summaryKey(target string, names []string, texts [][]byte) [32]byte {
	h := sha256.New()
	fmt.Fprintf(h, "summary 1\n%s\n%d\n", target, len(names))
	for i, n := range names {
		fmt.Fprintf(h, "%d:%s\n%d\n", len(n), n, len(texts[i]))
		h.Write(texts[i])
	}
	var k [32]byte
	h.Sum(k[:0])
	return k
}

func encodeSummary(texts [][]byte) []byte {
	var out []byte
	out = binary.AppendUvarint(out, uint64(len(texts)))
	for _, t := range texts {
		out = binary.AppendUvarint(out, uint64(len(t)))
		out = append(out, t...)
	}
	return out
}

func decodeSummary(data []byte, n int) ([][]byte, bool) {
	count, w := binary.Uvarint(data)
	if w <= 0 || count != uint64(n) {
		return nil, false
	}
	data = data[w:]
	out := make([][]byte, n)
	for i := range out {
		l, w := binary.Uvarint(data)
		if w <= 0 || uint64(len(data)-w) < l {
			return nil, false
		}
		out[i] = data[w : w+int(l)]
		data = data[w+int(l):]
	}
	return out, len(data) == 0
}

// prefetchImports starts asynchronous reading and parsing of unvisited imported folders.
func (l *importer) prefetchImports(f *ast.File, unit *token.File) {
	for _, stmt := range f.Stmts {
		decl, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		imp, ok := decl.D.(*ast.ImportDecl)
		if !ok || unit == nil {
			continue
		}
		for _, spec := range imp.Paths {
			path, ok := importPathText(spec.Path, unit)
			if !ok || path == "" {
				continue
			}
			dir, err := l.folder(path, filepath.Dir(unit.Name()))
			if err != nil || l.ahead[dir] != nil {
				continue
			}
			if _, loaded := l.loadedAt[dir]; loaded {
				continue
			}
			l.readAhead(dir)
		}
	}
}

// readFile is one source of an imported folder, read and parsed.
type readFile struct {
	src    Source
	unit   *token.File
	parsed *ast.File
	bad    bool  // the parser reported something
	err    error // the file could not be read
}

// parseSource parses an imported source and resolves its #if blocks.
func parseSource(src Source, cfg ifconfig.Config) readFile {
	tf := token.NewFile(src.Name, src.Text)
	parsed, ds := parser.ParseFile(tf, 0)
	if len(ds) > 0 {
		return readFile{src: src, bad: true}
	}
	ifconfig.Resolve(parsed, cfg)
	return readFile{src: src, unit: tf, parsed: parsed}
}

// bind binds the package whose Import is out[i] under the name as too: an
// importer's alias, or its package name where another importer aliased it.
func (l *importer) bind(i int, as string) {
	imp := &l.out[i]
	if imp.As == as || slices.Contains(imp.Aliases, as) {
		return
	}
	imp.Aliases = append(imp.Aliases, as)
}

// moduleNameOf returns the mangled module identifier for a package import path.
// Single-segment paths retain their name; deeper paths convert punctuation to underscores.
func moduleNameOf(path, name string) string {
	p := strings.Trim(path, "/")
	if !strings.Contains(p, "/") || strings.HasPrefix(p, ".") {
		return name
	}
	b := []byte(p)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			b[i] = '_'
		}
	}
	if b[0] >= '0' && b[0] <= '9' {
		return "_" + string(b)
	}
	return string(b)
}

// lastSegment returns the trailing folder name from a path.
func lastSegment(path string) string {
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
