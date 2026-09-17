package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/importer"
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
	// built is each package target's C-family dependencies, by the
	// target's directory, built once however often they are asked for.
	built map[string]*targetBuild
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
// checkout -replace names for it, or the checkout the importing file is in.
func (p *packages) Local(path, fromDir string) (string, error) {
	if dir, sub, ok := p.replaced(path); ok {
		return importer.Checkout(path, dir, sub)
	}
	return importer.Local(path, fromDir)
}

// Fetch is the directory of source for path, fetched into the cache if this
// machine does not have it, or "" for a path no repository answers to.
func (p *packages) Fetch(path string) (string, error) {
	if _, ok := importer.Lookup(path); !ok {
		return "", nil
	}
	return importer.Resolve(path, importer.Options{
		Offline: p.offline,
		Update:  p.update,
		Log:     p.notice,
	})
}

// TargetModules builds the C-family targets the package target in dir
// depends on, and is where their interfaces are. See vsc.TargetBuilder.
func (p *packages) TargetModules(dir string) (string, error) {
	t, err := (*common)(p).resolve()
	if err != nil {
		return "", err
	}
	b, err := (*common)(p).targets(dir, t)
	if err != nil || b == nil || len(b.objs) == 0 {
		return "", err
	}
	return b.modules, nil
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
		if !found || (path != name && !strings.HasPrefix(path, name+"/")) {
			continue
		}
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		return dir, strings.TrimPrefix(strings.TrimPrefix(path, name), "/"), true
	}
	return "", "", false
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
func sources(names []string) ([]vsc.Source, error) {
	if len(names) == 0 {
		names = []string{"-"}
	}
	names, err := expandDirs(names)
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
// it, in name order.
func expandDirs(names []string) ([]string, error) {
	var out []string
	for _, name := range names {
		info, err := os.Stat(name)
		if isStdout(name) || err != nil || !info.IsDir() {
			out = append(out, name)
			continue
		}
		files, err := filepath.Glob(filepath.Join(name, "*"+vsc.SourceExtension))
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("%s: no %s files in the folder", name, vsc.SourceExtension)
		}
		sort.Strings(files)
		out = append(out, files...)
	}
	return out, nil
}
