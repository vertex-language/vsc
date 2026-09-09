package vsc

import (
	"errors"
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/iface"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/lower"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/gen"
	"github.com/vertex-language/vsc/vil/pass"
)

// A Diagnostic is one message and the file it is about.
//
// token.Diagnostic carries a position but not a file, because a
// token.File owns a position space that starts at zero and knows
// nothing of any other -- so a position alone does not say which file
// it is in. Pairing them here is what lets a caller print a
// diagnostic from a compilation of several files.
//
// File is nil for a failure that is about the module rather than
// about a line: a pass refusing a program, or lowering refusing an
// instruction.
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

// Options are what the caller decides.
type Options struct {
	// Module is the name the module is compiled as. It is part of
	// every symbol, so two modules that share a name share symbols.
	Module string
	// Target is the machine to lower for.
	Target ir.Target
	// Stop says which phase to stop after. The zero value runs them
	// all.
	Stop Phase
	// PackagePaths are the roots a string-form import is looked for
	// in: `import "std/fmt"` is `<root>/std/fmt` in each, in order.
	// A path beginning `./` or `../` ignores these and resolves
	// against the importing file instead.
	PackagePaths []string
	// ImportPaths are the directories an `import` is looked for in.
	//
	// A module is found by name: `import Lib` looks for
	// Lib.vertexinterface in each, in order, and takes the first. The
	// interface is source, so what is found is parsed and checked the
	// way the program is -- see the iface package for why it is
	// source and not a binary format.
	ImportPaths []string
}

// A Phase is one step of the compiler, in the order they run.
type Phase int

const (
	// All runs every phase. It is the zero value because running the
	// whole compiler is what a caller usually wants.
	All Phase = iota
	// Parsed stops after the syntax tree.
	Parsed
	// Checked stops after names and types.
	Checked
	// Raw stops after the ownership IR is generated.
	Raw
	// Canonical stops after the passes that must run.
	Canonical
	// Lowered stops after ownership is erased.
	Lowered
)

// A Unit is what the phases produced. A field is nil where its phase
// did not run, either because the caller stopped before it or because
// a diagnostic did.
type Unit struct {
	// Files are the parsed files, in the order they were given.
	Files []*ast.File
	// Positions maps each file to the position table it was scanned
	// against, which is what turns a diagnostic into a line and
	// column.
	Positions []*token.File
	// Info is what the checker learned.
	Info *analyzer.Info
	// VIL is the ownership IR. Its stage says how far the passes got.
	VIL *vil.Module
	// VIR is the machine IR.
	VIR *ir.Module
	// Packages are the source folders this unit imported, in
	// dependency order -- a module after the ones whose names it
	// uses. Compiling and linking each is what makes the program
	// runnable, and is the caller's to do: this package produces one
	// module at a time, and a build of several is a build system.
	Packages []Package
}

// A Package is a folder of source that a program imported, and the
// module it is to be compiled as.
type Package struct {
	// Name is the module's name, which is what its symbols are
	// mangled with -- so it is what Options.Module has to be when
	// this package is compiled.
	Name string
	// Dir is where the folder was found, for a diagnostic that has to
	// say which one.
	Dir string
	// Sources are its files, in a stable order.
	Sources []Source
}

// Compile runs the phases in order and stops at the first one that
// reports an error.
//
// Diagnostics are returned rather than printed, and a warning does not
// stop anything: it is the caller's business how to show them and
// whether to care. What stops the compiler is an error, and it stops
// at the end of the phase that found it rather than at the first one
// -- so a file with three type errors reports three, and does not
// report the consequences of the first in the phase after.
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

	// The checker and the generator are given every file at once and
	// report against a position, and a position does not say which
	// file. Where there is one file there is no ambiguity; where there
	// are several, the message is still right and the line is not, so
	// it is left unattributed rather than attributed wrongly.
	imports, pkgs, importDiags := loadImports(u.Files, u.Positions, opts.ImportPaths, opts.PackagePaths)
	u.Packages = pkgs
	diags = append(diags, importDiags...)
	if Errors(diags) {
		return u, diags
	}
	info, checks := analyzer.CheckImporting(u.Files, imports)
	u.Info = info
	diags = append(diags, attribute(checks, u.only())...)
	if opts.Stop == Checked || Errors(diags) {
		return u, diags
	}

	m, gens := gen.Files(opts.Module, u.Files, info)
	u.VIL = m
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

	// Lowering needs a machine, and the zero Target is not one: it
	// describes nothing, and lowering against it produces diagnostics
	// about the program that are really diagnostics about the caller.
	// Saying so is worth more than the phase it saves.
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

// only is the one file this unit is of, or nil where there are
// several and a position cannot say which.
func (u *Unit) only() *token.File {
	if len(u.Positions) == 1 {
		return u.Positions[0]
	}
	return nil
}

// attribute pairs each diagnostic with the file it is about.
func attribute(ds []token.Diagnostic, f *token.File) []Diagnostic {
	out := make([]Diagnostic, len(ds))
	for i, d := range ds {
		out[i] = Diagnostic{Diagnostic: d, File: f}
	}
	return out
}

// phaseError turns a failure with no position -- one a pass or the
// lowering reported about the module rather than about a line -- into
// a diagnostic, so that a caller has one kind of thing to report.
func phaseError(err error) Diagnostic {
	return Diagnostic{Diagnostic: token.Diagnostic{
		Pos:      token.NoPos,
		End:      token.NoPos,
		Severity: token.Error,
		Message:  err.Error(),
	}}
}

// errNoTarget is what Compile reports when it is asked to lower for
// no machine. Stopping at Canonical or earlier needs no target and
// does not reach this.
var errNoTarget = errors.New("no target: lowering needs a machine to lower for")

// loadImports finds and reads what a program imports, and what those
// imports import in turn.
//
// A module is found by name in the search path and parsed with the
// ordinary parser, because an interface is ordinary source. A module
// named twice is read once: two files importing the same library is
// the common case, not a mistake.
//
// The reading is transitive, and has to be. An interface names types
// from the modules it imports -- Foundation's says `Swift.Int`, and a
// framework's says `Foundation.Data` -- so a module read without its
// own imports is a module whose signatures mostly do not resolve. The
// order matters too: a module is put in the list after the ones it
// imports, so that by the time its own declarations are read, the
// names they use are there.
//
// An import that cannot be found is an error against the line that
// wrote it, and one reached through another module says which module
// asked for it. Nothing used to be: `import Anything` was accepted
// and ignored, so a program that imported a module that was not there
// failed later, on every name it expected to find in it.
func loadImports(files []*ast.File, units []*token.File, paths, pkgPaths []string) ([]analyzer.Import, []Package, []Diagnostic) {
	l := &importer{paths: paths, pkgPaths: pkgPaths, seen: map[string]bool{}}
	for i, f := range files {
		unit := f.Unit
		if i < len(units) && units[i] != nil {
			unit = units[i]
		}
		l.readAll(f, unit, "")
	}
	return l.out, l.pkgs, l.diags
}

// An importer reads a module and everything it stands on.
type importer struct {
	paths    []string
	pkgPaths []string
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

// builtinModule reports whether a module is one this compiler already
// is, so that importing it loads nothing.
//
// `import Swift` names the universe: Int and Bool and the rest are
// here before any file is read, and there is no interface to find
// because there is no separate module to read one from. The
// underscored four are the standard library's own pieces, and every
// interface swiftc emits imports them whether or not it uses them --
// so treating them as anything but built in would put three
// diagnostics on every module in the SDK. What they declare and this
// compiler does not have is reported where it is used, by name, which
// is where a reader can do something about it.
func builtinModule(name string) bool {
	switch name {
	case "Swift", "Builtin", "_Concurrency", "_StringProcessing",
		"_SwiftConcurrencyShims", "_math":
		return true
	}
	return false
}

// read reads one module, then the modules it imports, then adds it.
func (l *importer) read(name string, at *ast.ImportDecl, unit *token.File, via string) {
	// A module the program named itself is the program's business,
	// and not finding it is an error. A module reached through
	// another module's interface is that module's business: the
	// program may never touch anything from it, and stopping the
	// compilation over a module nobody asked for would make importing
	// anything real impossible. So that is a warning, and what the
	// program does use from an unread module fails by name where it
	// is used.
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
	// What the module's own compiler said about it. A library built
	// with library evolution has an ABI this compiler does not
	// generate: its types have no layout a client may rely on, a
	// field is a getter call rather than an offset, and a value
	// crosses a call through its value witnesses. Reading its
	// interface and compiling against it the way this compiler
	// compiles anything else produces a program that links and then
	// dies on the first field read, so it is refused here instead.
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
	// Its own imports first, so that the list is in the order the
	// checker needs: a module after the ones whose names it uses.
	l.readAll(parsed, tf, name)
	l.out = append(l.out, analyzer.Import{
		Name:  name,
		Files: []*ast.File{parsed},
		Units: []*token.File{tf},
	})
}

// findInterface looks for a module's interface in the search path.
//
// Three layouts, because three things write one. This compiler writes
// `<Module>.vertexinterface` beside the object. swiftc writes
// `<Module>.swiftinterface` when it is asked for one by path. And a
// built module -- an SDK framework, a package's build directory --
// is a `<Module>.swiftmodule` directory with one interface per
// target inside it, which is where the interface actually lives for
// anything that was not built by hand.
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

// inSwiftModule is the interface for this machine inside a built
// module directory.
//
// The files there are named for the target they were built for --
// `arm64e-apple-macos.swiftinterface` beside `x86_64-apple-macos.swiftinterface`
// -- so the one to read is the one whose name starts with this
// machine's architecture. A `.private.swiftinterface` beside it
// describes the same module including what is not public, and is not
// what a client reads.
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

// importPathText is the text of a string-form import path.
//
// A path is a plain string: one run of text, no interpolation. That
// is not a restriction anyone will feel -- a folder name is not
// computed -- and it means the path is read without the escape
// decoding a general string literal needs.
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

// findFolder resolves an import path to a directory.
//
// A path beginning `./` or `../` is relative to the file that wrote
// it, which is the marked form: it says "not from the package
// directory", and is what local work and tests use. Everything else
// is looked for under each package root in order, which is where a
// package manager leaves what it downloads.
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

// packageNameOf is the module a folder's files declare, or "" where
// none of them says.
//
// One file saying is enough, and two disagreeing is the folder's
// mistake rather than the importer's: it is reported where the module
// is built, not at every program that imports it.
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

// SourceExtension is what a Vertex source file is called. A folder
// import reads every one of these in the directory it names.
const SourceExtension = ".vs"

// readFolder reads one string-form import: a folder of source files,
// checked as the module they declare.
//
// The folder's files are parsed and handed to the checker the way an
// interface is, because an interface is source with the bodies taken
// out -- the same passes run over both, and the bodies are not
// checked either way. So a folder of source needs nothing of the
// checker that an interface did not already need.
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
	dir, found := findFolder(path, filepath.Dir(unit.Name()), l.pkgPaths)
	if !found {
		where := "the package directory"
		if strings.HasPrefix(path, ".") {
			where = filepath.Dir(unit.Name())
		}
		fail("no such package '" + path + "': looked in " + where)
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

	// The module's identity is what its files declare, or its
	// folder's name where they declare nothing -- the convention
	// SwiftPM already follows, with the clause as the override for a
	// directory whose name is not an identifier.
	name := packageNameOf(files, units)
	if name == "" {
		name = lastSegment(path)
	}
	if name == "" {
		fail("cannot name the module for '" + path + "': add a package declaration")
		return
	}
	// A rename says what this file calls the module. It does not
	// change what the module is: its symbols are mangled with its own
	// name, so two modules that share one still collide at the link.
	as := name
	if spec.Alias != nil {
		as = spec.Alias.Text(unit)
	}
	if l.seen[as] {
		return
	}
	l.seen[as] = true

	// Its own imports first, so that the list is in the order the
	// checker needs: a module after the ones whose names it uses.
	for i, f := range files {
		l.readAll(f, units[i], name)
	}
	l.out = append(l.out, analyzer.Import{Name: name, As: as, Files: files, Units: units})
	// After its own imports, so the list a caller compiles is in the
	// order it has to compile them.
	l.pkgs = append(l.pkgs, Package{Name: name, Dir: dir, Sources: srcs})
}

// lastSegment is the final component of an import path, which is the
// folder's own name and so the module's.
func lastSegment(path string) string {
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
