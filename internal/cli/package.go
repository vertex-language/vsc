package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/pkg"
	"github.com/vertex-language/vsc/token"
)

// isPackageBuild reports whether a build's arguments ask for a package, and
// for which product: no arguments, or one that names no file.
func isPackageBuild(bf *buildFlags, names []string) (root, product string, ok bool) {
	switch len(names) {
	case 0:
	case 1:
		if filepath.Ext(names[0]) != "" {
			return "", "", false
		}
		// A name that is a path is that path -- unless the manifest here
		// declares a product of the name, which is what it means then:
		// `vsc build rdpviewer` beside a folder rdpviewer/.
		if _, err := os.Stat(names[0]); err == nil && !declaresProduct(bf, names[0]) {
			return "", "", false
		}
		product = names[0]
	default:
		return "", "", false
	}
	dir := bf.packagePath
	if dir == "" {
		dir = "."
	}
	if _, ok := pkg.FindManifest(dir); !ok {
		return "", "", false
	}
	return dir, product, true
}

// declaresProduct reports whether the manifest in the package directory
// declares a product named name.
func declaresProduct(bf *buildFlags, name string) bool {
	dir := bf.packagePath
	if dir == "" {
		dir = "."
	}
	if _, ok := pkg.FindManifest(dir); !ok {
		return false
	}
	m, _, err := pkg.Load(dir)
	if err != nil || m == nil {
		return false
	}
	for _, p := range m.Products {
		if p.Name == name {
			return true
		}
	}
	return false
}

// doPackageBuild builds the package at root and writes its programs, or the
// one product named. It returns where the program landed when there is one to run.
//
// A program that imports by path -- `import "net/tcp"` -- is built as a
// program of files is: its target's sources are compiled, and what they
// import is found the way any import is (see vsc's importer). A library of
// the same package is the checkout the program is in, so it is read from
// disk, with the C-family targets the manifest has it depend on built
// beside it. The manifest is only needed for those: a package of Vertex
// alone has none.
//
// A program that imports its package's targets by module name --
// `import tcp`, as a SwiftPM package does -- is built the way SwiftPM
// builds it: every target, in the manifest's order. See build.BuildPackage.
func doPackageBuild(bf *buildFlags, mode emitMode, root, product string, target ir.Target, stdout, stderr io.Writer) (string, int) {
	if mode.name != "exe" {
		fmt.Fprintf(stderr, "vsc: a package builds programs; --emit %s is for files\n", mode.name)
		return "", exitUsage
	}
	m, mdiags, err := pkg.Load(root)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	diags := make([]vsc.Diagnostic, len(mdiags))
	for i, d := range mdiags {
		diags[i] = vsc.Diagnostic{Diagnostic: d, File: d.File}
	}
	if printDiags(stderr, diags) {
		return "", exitDiags
	}
	platform := pkg.PlatformOf(target.Use())
	p, err := pkg.Resolve(root, m, platform, "debug")
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitDiags
	}
	programs := p.Executables()
	if len(programs) == 0 {
		fmt.Fprintf(stderr, "vsc: package '%s' has no executable products\n", m.Name)
		return "", exitUsage
	}
	var names []string
	for _, prog := range programs {
		names = append(names, prog.Name)
	}
	if product != "" {
		found := false
		for _, n := range names {
			found = found || n == product
		}
		if !found {
			fmt.Fprintf(stderr, "vsc: no executable product named '%s' (the package has: %s)\n",
				product, strings.Join(names, ", "))
			return "", exitUsage
		}
	}
	// An output path is one program's: run's, or -o's.
	if bf.output != "" && product == "" && len(programs) > 1 {
		fmt.Fprintf(stderr, "vsc: package '%s' has several executables (%s): name the one to build\n",
			m.Name, strings.Join(names, ", "))
		return "", exitUsage
	}
	work := filepath.Join(p.Root, ".build", "vsc", "debug")
	if err := os.MkdirAll(work, 0o755); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	landed := ""
	var swiftpm map[string][]byte
	for _, prog := range programs {
		if product != "" && prog.Name != product {
			continue
		}
		var files []string
		for _, src := range prog.Target.Sources {
			if src.Language == pkg.Swift {
				files = append(files, src.Path)
			}
		}
		if len(files) == 0 {
			fmt.Fprintf(stderr, "vsc: the program '%s' has no Vertex sources in %s\n", prog.Name, prog.Target.Dir)
			return "", exitUsage
		}
		out := vsc.ImageName(target, filepath.Join(work, prog.Name))
		if bf.output != "" {
			out = vsc.ImageName(target, bf.output)
		}
		if !importsByPath(files) {
			if swiftpm == nil {
				images, code := buildSwiftPM(p, target, work, stderr)
				if code != exitOK {
					return "", code
				}
				swiftpm = images
			}
			if code := write(out, io.Discard, stderr, swiftpm[prog.Name], true); code != exitOK {
				return "", code
			}
			landed = out
			continue
		}
		one := *bf
		one.module = vsc.EntryModule
		one.output = out
		out, code := doFilesBuild(&one, mode, files, target, stdout, stderr)
		if code != exitOK {
			return "", code
		}
		// What one program built -- a C target, a fetched package -- the next
		// one uses as it is.
		bf.built = one.built
		landed = out
	}
	return landed, exitOK
}

// importsByPath reports whether any of the files imports a package by path,
// which is how a Vertex program imports; a SwiftPM one imports by module name.
func importsByPath(files []string) bool {
	for _, name := range files {
		text, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		f, _ := parser.ParseFile(token.NewFile(name, text), 0)
		if f == nil {
			continue
		}
		for _, stmt := range f.Stmts {
			if d, ok := stmt.(*ast.DeclStmt); ok {
				if imp, ok := d.D.(*ast.ImportDecl); ok && len(imp.Paths) > 0 {
					return true
				}
			}
		}
	}
	return false
}

// buildSwiftPM builds every target of a SwiftPM-style package and links
// its programs, by product name.
func buildSwiftPM(p *pkg.Package, target ir.Target, work string, stderr io.Writer) (map[string][]byte, int) {
	products, err := build.BuildPackage(p, build.PackageOptions{Target: target, Config: "debug", Work: work})
	if err != nil {
		var pe *build.PackageError
		if errors.As(err, &pe) && len(pe.Diags) > 0 {
			printDiags(stderr, pe.Diags)
			return nil, exitDiags
		}
		fmt.Fprintln(stderr, "vsc:", err)
		return nil, exitDiags
	}
	images := map[string][]byte{}
	for _, prod := range products {
		images[prod.Name] = prod.Image
	}
	return images, exitOK
}
