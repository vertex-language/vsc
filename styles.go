package vsc

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vss"
)

// styleSource is the generated source a package with .vss files carries:
// its stylesheet, as vss.Source makes it. pkg is the package's name as its
// Vertex files give it ("" for a package of styles alone), and imports are
// the paths its Vertex files import. The sheets it comes after are those
// of the packages its .vss and Vertex files import that have .vss files of
// their own. It is nil where the folder has no .vss files.
func styleSource(dir, pkg string, imports []string, resolve func(path, fromDir string) (string, error)) (*Source, []Diagnostic) {
	names, _ := filepath.Glob(filepath.Join(dir, "*"+vss.Extension))
	if len(names) == 0 {
		return nil, nil
	}
	sort.Strings(names)
	var diags []Diagnostic
	var files []*vss.File
	paths := append([]string(nil), imports...)
	for _, name := range names {
		text, err := os.ReadFile(name)
		if err != nil {
			diags = append(diags, Diagnostic{Diagnostic: token.Diagnostic{Severity: token.Error,
				Message: "cannot read '" + name + "': " + err.Error()}})
			continue
		}
		unit := token.NewFile(name, text)
		f, ds := vss.Parse(unit)
		diags = append(diags, attribute(ds, unit)...)
		if f.Package == "" {
			continue
		}
		if pkg == "" {
			pkg = f.Package
		} else if f.Package != pkg {
			diags = append(diags, Diagnostic{File: unit, Diagnostic: token.Diagnostic{Pos: unit.Pos(0), End: unit.Pos(0),
				Severity: token.Error, File: unit,
				Message: "this .vss file is package " + f.Package + ", and its folder is package " + pkg}})
			continue
		}
		for _, imp := range f.Imports {
			paths = append(paths, imp.Path)
		}
		files = append(files, f)
	}
	if len(files) == 0 || Errors(diags) {
		return nil, diags
	}
	// The imports with styles of their own, each once, and the tokens
	// each exports.
	seen := map[string]bool{}
	var after []string
	tokens := map[string]map[string]bool{pkg: vss.Declared(files)}
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true
		d, err := resolve(p, dir)
		if err != nil || d == "" || filepath.Clean(d) == filepath.Clean(dir) {
			continue
		}
		if styled, _ := filepath.Glob(filepath.Join(d, "*"+vss.Extension)); len(styled) > 0 {
			after = append(after, p)
			if name, exported := exportedTokens(styled); name != "" {
				tokens[name] = exported
			}
		}
	}
	// What the engine would not do with the styles: an unknown property,
	// one it does not apply yet, a library restyling the document, a token
	// no package declares.
	for _, f := range files {
		diags = append(diags, attribute(vss.Check(f, pkg == "main", tokens), f.Unit)...)
	}
	if Errors(diags) {
		return nil, diags
	}
	text := vss.Source(pkg, vss.Emit(pkg, files), after, vss.Registered(files))
	return &Source{Name: filepath.Join(dir, "__vss_"+pkg+".vs"), Text: []byte(text)}, diags
}

// importPaths are the string-form import paths the files name.
func importPaths(files []*ast.File, units []*token.File) []string {
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
				if p, ok := importPathText(spec.Path, unit); ok && p != "" {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// programStyles are the generated sources of the program's own folders
// that have .vss files: the folders its source files are in.
func programStyles(files []*ast.File, units []*token.File, srcs []Source, opts Options) ([]Source, []Diagnostic) {
	l := &importer{packages: opts.Packages, pkgPaths: opts.PackagePaths, seen: map[string]bool{}}
	byDir := map[string][]int{}
	var dirs []string
	// A package built on its own is given its generated source with the
	// rest: it is not made twice.
	made := map[string]bool{}
	for _, src := range srcs {
		if strings.HasPrefix(filepath.Base(src.Name), "__vss_") {
			made[filepath.Dir(src.Name)] = true
		}
	}
	for i, src := range srcs {
		if made[filepath.Dir(src.Name)] {
			continue
		}
		if _, err := os.Stat(src.Name); err != nil {
			continue // a source not on disk has no folder of styles
		}
		dir := filepath.Dir(src.Name)
		if _, ok := byDir[dir]; !ok {
			dirs = append(dirs, dir)
		}
		byDir[dir] = append(byDir[dir], i)
	}
	var out []Source
	var diags []Diagnostic
	for _, dir := range dirs {
		var fs []*ast.File
		var us []*token.File
		for _, i := range byDir[dir] {
			fs = append(fs, files[i])
			us = append(us, units[i])
		}
		gen, ds := styleSource(dir, packageNameOf(fs, us), importPaths(fs, us), l.folder)
		diags = append(diags, ds...)
		if gen != nil {
			out = append(out, *gen)
		}
	}
	return out, diags
}

// exportedTokens are the tokens an imported package's .vss files export,
// and the package's name; its files' own problems are reported where it
// is built.
func exportedTokens(names []string) (string, map[string]bool) {
	var files []*vss.File
	pkg := ""
	for _, name := range names {
		text, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		f, _ := vss.Parse(token.NewFile(name, text))
		if f.Package == "" {
			continue
		}
		pkg = f.Package
		files = append(files, f)
	}
	return pkg, vss.Tokens(files)
}
