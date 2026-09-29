package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/doc"
	"github.com/vertex-language/vsc/importer"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/pkg"
	"github.com/vertex-language/vsc/token"
)

// cmdDoc writes a package's documentation: its exported declarations'
// signatures and comments, as Markdown or as an HTML page.
func cmdDoc(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	format := fs.String("format", "", "md or html (default: from -o's extension, else md)")
	output := fs.String("o", "", "write the documentation here (default: standard output)")
	all := fs.Bool("all", false, "document every declaration, not only public and open ones")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "vsc: doc documents one package: vsc doc [flags] [package]")
		return exitUsage
	}
	f := *format
	if f == "" {
		switch strings.ToLower(filepath.Ext(*output)) {
		case ".html", ".htm":
			f = "html"
		default:
			f = "md"
		}
	}
	if f == "markdown" {
		f = "md"
	}
	if f != "md" && f != "html" {
		fmt.Fprintf(stderr, "vsc: unknown -format %q (known: md, html)\n", f)
		return exitUsage
	}
	target, err := c.resolve()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	c.notice = func(msg string) { fmt.Fprintln(stderr, "vsc:", msg) }

	arg := "."
	if fs.NArg() == 1 {
		arg = fs.Arg(0)
	}
	dir, path, err := c.docDir(arg)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	use := target.Use()
	folder, err := pkg.ReadFolder(dir, pkg.PlatformOf(use), pkg.ArchOf(use))
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	var srcs []vsc.Source
	for _, name := range folder.Vertex {
		s, err := source(name)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return exitUsage
		}
		srcs = append(srcs, s)
	}
	// A folder of C++ alone exports what its module does: the Vertex
	// source binding gives it is the package's API.
	if len(folder.Vertex) == 0 && len(folder.Native) > 0 {
		c.mainModule(dir)
		bound, err := c.bind(dir, path, filepath.Base(dir), true, target)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return exitDiags
		}
		srcs = append(srcs, bound...)
	}
	if len(srcs) == 0 {
		fmt.Fprintf(stderr, "vsc: %s: no %s files in the folder\n", dir, vsc.SourceExtension)
		return exitUsage
	}

	var files []*ast.File
	var units []*token.File
	for _, s := range srcs {
		unit := token.NewFile(s.Name, s.Text)
		// A file that does not parse cleanly is still documented as far
		// as it does: documentation is for reading, not for building.
		f, _ := parser.ParseFile(unit, parser.ParseComments|parser.SkipBodies|parser.Tolerant)
		files = append(files, f)
		units = append(units, unit)
	}
	p := doc.New(files, units, doc.Options{All: *all, ImportPath: path})
	if p.Name == "" {
		p.Name = filepath.Base(dir)
	}
	// A program is not imported: its page is the command's, as godoc's is.
	if p.Name == "main" {
		p.ImportPath = ""
	}

	var buf bytes.Buffer
	if f == "html" {
		err = doc.HTML(&buf, p)
	} else {
		err = doc.Markdown(&buf, p)
	}
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	return write(*output, stdout, stderr, buf.Bytes(), false)
}

// docDir is the folder a package argument names and the path it is
// imported by: a folder on disk, or an import path found as a build finds
// one -- in this checkout, a -replace, or the package cache, fetching it
// if need be.
func (c *common) docDir(arg string) (dir, path string, err error) {
	if info, statErr := os.Stat(arg); statErr == nil && info.IsDir() {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", "", err
		}
		return abs, importPathOf(abs), nil
	}
	wd, _ := os.Getwd()
	p := (*packages)(c)
	if dir, err := p.Local(arg, wd); err != nil || dir != "" {
		return dir, arg, err
	}
	for _, root := range c.packagePaths() {
		d := filepath.Join(root, filepath.FromSlash(arg))
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return d, arg, nil
		}
	}
	c.mainModule(wd)
	dir, err = p.Fetch(arg)
	if err != nil {
		return "", "", err
	}
	if dir == "" {
		return "", "", fmt.Errorf("no package %q: not a folder here, and not an import path (like %s)",
			arg, strings.Join(importer.StdlibExamples(), ", "))
	}
	return dir, strings.SplitN(arg, "@", 2)[0], nil
}

// importPathOf is the path a folder on disk is imported by: its
// checkout's own path and the folder's place in it, "" where it is in no
// checkout.
func importPathOf(dir string) string {
	root, ok := importer.Enclosing(dir)
	if !ok {
		return ""
	}
	rel, err := filepath.Rel(root.Dir, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	path := root.Path
	if rel != "." {
		path += "/" + filepath.ToSlash(rel)
	}
	// The standard library is imported by its short path: net/tcp, not
	// github.com/vertex-language/net/tcp.
	return strings.TrimPrefix(path, strings.TrimPrefix(importer.StdlibOrg, "https://")+"/")
}
