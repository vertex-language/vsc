package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/importer"
	"github.com/vertex-language/vsc/pkg"
)

// common is the flag set every verb shares.
type common struct {
	target  string
	module  string
	include includePath
	pkgs    includePath
	offline bool
	update  bool
	replace includePath
	// notice reports what a build is doing that it might otherwise seem
	// to hang on -- fetching a package. Set by the command; nil is quiet.
	notice func(string)
	// main is the module being built: the checkout the program's files
	// are in, with its vs.mod, and the vs.work above it.
	main *mainModule
	// natives are the C++ modules of the folders read so far, by
	// directory, bound once however often they are imported.
	natives map[string]*native
}

// includePath collects repeated -I or -P flags in order.
type includePath []string

func (p *includePath) String() string { return strings.Join(*p, ", ") }
func (p *includePath) Set(v string) error {
	*p = append(*p, v)
	return nil
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.target, "target", vsc.HostName(), "target to build for")
	fs.StringVar(&c.module, "module", vsc.EntryModule, "the module being compiled")
	fs.Var(&c.include, "I", "a directory to look for imported modules in (repeatable)")
	fs.Var(&c.pkgs, "P", "a package directory root, for string-form imports (repeatable)")
	fs.BoolVar(&c.offline, "offline", false, "never fetch a package: use the cache as it is")
	fs.BoolVar(&c.update, "update", false, "fetch imported packages again, even if the cache holds them")
	fs.Var(&c.replace, "replace", "use a local checkout for an imported package: path=dir (repeatable)")
}

// packagePaths returns search roots from -P flags followed by VERTEXPATH.
func (c *common) packagePaths() []string {
	out := append([]string{}, c.pkgs...)
	for _, dir := range filepath.SplitList(os.Getenv("VERTEXPATH")) {
		if dir != "" {
			out = append(out, dir)
		}
	}
	return out
}

// resolve validates and returns the target specified by -target.
func (c *common) resolve() (ir.Target, error) {
	if c.target == "" {
		return ir.Target{}, fmt.Errorf("this host is not a target vsc models; name one with -target (known: %s)",
			strings.Join(vsc.Targets(), ", "))
	}
	t, ok := vsc.LookupTarget(c.target)
	if !ok {
		return ir.Target{}, fmt.Errorf("unknown target %q (known: %s)",
			c.target, strings.Join(vsc.Targets(), ", "))
	}
	return t, nil
}

// options constructs vsc.Options for compilation.
func (c *common) options(t ir.Target, stop vsc.Phase) vsc.Options {
	return vsc.Options{
		Module:       c.module,
		Target:       t,
		Stop:         stop,
		ImportPaths:  c.include,
		PackagePaths: c.packagePaths(),
		Packages:     (*packages)(c),
	}
}

// A packages resolves import paths for the compiler by fetching them.
// See vsc/importer.
type packages common

// Local is the directory of source for path without fetching: the
// checkout -replace names for it, one a vs.work uses or the main module's
// vs.mod replaces it with, or the checkout the importing file is in.
func (p *packages) Local(path, fromDir string) (string, error) {
	if dir, sub, ok := p.replaced(path); ok {
		return importer.Checkout(path, dir, sub)
	}
	if m := (*common)(p).mainModule(fromDir); m != nil {
		if dir, sub, ok := m.local(path); ok {
			return importer.Checkout(path, dir, sub)
		}
	}
	return importer.Local(path, fromDir)
}

// Fetch is the directory of source for path, fetched into the cache if this
// machine does not have it, or "" for a path no repository answers to. The
// version is the one the main module's vs.mod requires, and what is
// fetched at it must hash as its vs.sum records.
func (p *packages) Fetch(path string) (string, error) {
	if _, ok := importer.Lookup(path); !ok {
		return "", nil
	}
	m := (*common)(p).main
	version := ""
	if m != nil && m.root.Mod != nil {
		module := importer.ModuleOf(path)
		version = m.root.Mod.Required(module)
		// A replace with another module fetches that one, at its version,
		// in the path's place.
		for _, r := range m.replaces {
			if r.Dir != "" || r.Module != module || (r.Version != "" && r.Version != version) {
				continue
			}
			sub, _ := under(path, module)
			path = r.New.Module
			if sub != "" {
				path += "/" + sub
			}
			version = r.New.Version
			break
		}
	}
	dir, err := importer.Resolve(path, importer.Options{
		Ref:     version,
		Offline: p.offline,
		Update:  p.update,
		Log:     p.notice,
	})
	if err != nil || version == "" {
		return dir, err
	}
	return dir, m.check(importer.ModuleOf(path), version, dir, path)
}

// replaced is a directory named by -replace for this path. It is how a
// package is worked on: the checkout being edited stands in for the one
// that would be fetched, without the program importing it changing. A
// replacement names a checkout, so `-replace net=../net` stands in for
// net/tcp and net/udp as well as net; what is left of the path after the
// name is the folder inside it.
func (p *packages) replaced(path string) (dir, sub string, ok bool) {
	for _, r := range p.replace {
		name, dir, found := strings.Cut(r, "=")
		if !found {
			continue
		}
		if sub, ok := under(path, name); ok {
			if abs, err := filepath.Abs(dir); err == nil {
				dir = abs
			}
			return dir, sub, true
		}
	}
	return "", "", false
}

// under reports whether path is module or a folder of it, and which.
func under(path, module string) (sub string, ok bool) {
	if path == module {
		return "", true
	}
	if strings.HasPrefix(path, module+"/") {
		return strings.TrimPrefix(path, module+"/"), true
	}
	return "", false
}

// source reads one input file or standard input ("" or "-").
func source(name string) (vsc.Source, error) {
	if isStdout(name) {
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			return vsc.Source{}, fmt.Errorf("<stdin>: %w", err)
		}
		return vsc.Source{Name: "<stdin>", Text: src}, nil
	}
	src, err := os.ReadFile(name)
	if err != nil {
		return vsc.Source{}, err
	}
	return vsc.Source{Name: name, Text: src}, nil
}

// sources reads multiple input files in order, defaulting to stdin if empty.
// A directory stands for the Vertex files in it, as a folder is a package:
// `vsc run tests/all`.
func sources(names []string, target ir.Target) ([]vsc.Source, error) {
	if len(names) == 0 {
		names = []string{"-"}
	}
	names, err := expandDirs(names, target)
	if err != nil {
		return nil, err
	}
	out := make([]vsc.Source, 0, len(names))
	for _, name := range names {
		s, err := source(name)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// outputName derives the default output filename from the first source file.
func outputName(srcs []vsc.Source, ext string) string {
	base := "a"
	if len(srcs) > 0 && srcs[0].Name != "<stdin>" {
		base = strings.TrimSuffix(filepath.Base(srcs[0].Name), filepath.Ext(srcs[0].Name))
	}
	return base + ext
}

// expandDirs replaces each directory among names with the Vertex files in
// it that the target builds, in name order: a folder is a package.
func expandDirs(names []string, target ir.Target) ([]string, error) {
	var out []string
	for _, name := range names {
		info, err := os.Stat(name)
		if isStdout(name) || err != nil || !info.IsDir() {
			out = append(out, name)
			continue
		}
		f, err := pkg.ReadFolder(name, pkg.PlatformOf(target.Use()), pkg.ArchOf(target.Use()))
		if err != nil {
			return nil, err
		}
		if len(f.Vertex) == 0 {
			return nil, fmt.Errorf("%s: no %s files in the folder", name, vsc.SourceExtension)
		}
		out = append(out, f.Vertex...)
	}
	return out, nil
}
