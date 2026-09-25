package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/importer"
	"github.com/vertex-language/vsc/pkg"
)

// A native is a folder's C++ module as bound for one build: the module,
// the thunk unit its Vertex declarations call, those declarations, and the
// folders whose modules its C++ imports.
type native struct {
	n      *build.Native
	thunks []byte
	srcs   []vsc.Source
	deps   []string
}

// NativeSources reads the C++ module of the folder dir, and is the Vertex
// declarations its exports are. See vsc.NativeBinder.
func (p *packages) NativeSources(dir, path, pkgName string, public bool) ([]vsc.Source, error) {
	c := (*common)(p)
	target, err := c.resolve()
	if err != nil {
		return nil, err
	}
	nat, err := c.bind(dir, path, pkgName, public, target)
	if err != nil {
		return nil, err
	}
	return nat, nil
}

// bind reads the C++ module in dir once per build.
func (c *common) bind(dir, path, pkgName string, public bool, target ir.Target) ([]vsc.Source, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if nat, ok := c.natives[abs]; ok {
		if nat == nil {
			return nil, fmt.Errorf("%s: its C++ module imports itself, through other modules", abs)
		}
		return nat.srcs, nil
	}
	use := target.Use()
	folder, err := pkg.ReadFolder(abs, pkg.PlatformOf(use), pkg.ArchOf(use))
	if err != nil {
		return nil, err
	}
	if len(folder.Native) == 0 {
		return nil, nil
	}
	n, err := build.FindNative(abs, folder.Native, target)
	if err != nil {
		return nil, err
	}
	// A folder imported by path declares the module its path is, as a Go
	// package's clause names its folder: net/tcp is net.tcp.
	if path != "" && !strings.HasPrefix(path, ".") {
		if want := build.ModuleNameFor(path); n.Module != want {
			return nil, fmt.Errorf("%s declares module %s, but a package imported as \"%s\" is module %s: write `export module %s;`",
				filepath.Base(n.Interface), n.Module, path, want, want)
		}
	}
	if c.natives == nil {
		c.natives = map[string]*native{}
	}
	c.natives[abs] = nil // being bound
	nat := &native{n: n}

	// The modules its C++ imports: another package's, which is bound (and
	// linked) as this one is, or the runtime's task ABI.
	n.Modules = map[string]string{}
	for _, name := range n.Imports {
		if name == build.TaskModule {
			file, err := build.TaskModuleFile(nativeWork())
			if err != nil {
				return nil, err
			}
			n.Modules[name] = file
			continue
		}
		ipath := c.importPathOf(name)
		ddir, err := c.packageDir(ipath, abs)
		if err != nil {
			return nil, fmt.Errorf("%s imports module %s: %w", filepath.Base(n.Interface), name, err)
		}
		dabs, _ := filepath.Abs(ddir)
		dfolder, err := pkg.ReadFolder(dabs, pkg.PlatformOf(use), pkg.ArchOf(use))
		if err != nil {
			return nil, err
		}
		if _, err := c.bind(dabs, ipath, lastSegment(ipath), len(dfolder.Vertex) == 0, target); err != nil {
			return nil, err
		}
		dep := c.natives[dabs]
		if dep == nil {
			return nil, fmt.Errorf("%s imports module %s, which has no C++ in %s", filepath.Base(n.Interface), name, dabs)
		}
		n.Modules[name] = dep.n.Interface
		for k, v := range dep.n.Modules {
			n.Modules[k] = v
		}
		nat.deps = append(nat.deps, dabs)
	}

	vs, thunks, skipped, err := n.Bindings(pkgName, public)
	if err != nil {
		return nil, err
	}
	if c.notice != nil {
		for _, s := range skipped {
			c.notice(fmt.Sprintf("C++ module %s: not imported: %s", n.Module, s))
		}
	}
	nat.thunks = thunks
	nat.srcs = []vsc.Source{{Name: filepath.Join(abs, "__vs_cxx_"+strings.ReplaceAll(n.Module, ".", "_")+".vs"), Text: vs}}
	c.natives[abs] = nat
	return nat.srcs, nil
}

// importPathOf is the import path of the package a C++ module name is:
// a folder of the main module where its name begins with the main
// module's (proj.math in github.com/me/proj is github.com/me/proj/math),
// and otherwise the standard library's, dots for slashes (net.tcp is
// net/tcp).
func (c *common) importPathOf(module string) string {
	if c.main != nil && c.main.root.Path != "" {
		own := build.ModuleNameFor(c.main.root.Path)
		if module == own {
			return c.main.root.Path
		}
		if strings.HasPrefix(module, own+".") {
			return c.main.root.Path + "/" + strings.ReplaceAll(strings.TrimPrefix(module, own+"."), ".", "/")
		}
	}
	return strings.ReplaceAll(module, ".", "/")
}

// packageDir finds the folder an import path names, as an import in a
// file in fromDir would: on disk first, then fetched.
func (c *common) packageDir(path, fromDir string) (string, error) {
	p := (*packages)(c)
	dir, err := p.Local(path, fromDir)
	if err != nil || dir != "" {
		return dir, err
	}
	dir, err = p.Fetch(path)
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", fmt.Errorf("no package %s", path)
	}
	return dir, nil
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// nativeWork is where native objects, thunks and generated modules go.
func nativeWork() string { return filepath.Join(importer.CacheDir(), "build") }

// nativeObjects compiles the C++ module bound for dir, and those of the
// modules its C++ imports, and says what the link needs for them. Nil
// where the folder has none.
func (c *common) nativeObjects(dir string) ([]build.Input, build.Linkage, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, build.Linkage{}, err
	}
	var objs []build.Input
	var need build.Linkage
	seen := map[string]bool{}
	var walk func(string) error
	walk = func(d string) error {
		if seen[d] {
			return nil
		}
		seen[d] = true
		nat := c.natives[d]
		if nat == nil {
			return nil
		}
		o, n, err := nat.n.Objects(nat.thunks, nativeWork())
		if err != nil {
			return err
		}
		objs = append(objs, o...)
		need = need.Merge(n)
		for _, dep := range nat.deps {
			if err := walk(dep); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(abs); err != nil {
		return nil, need, err
	}
	return objs, need, nil
}
