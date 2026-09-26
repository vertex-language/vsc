package cli

import (
	"encoding/json"
	"fmt"
	"github.com/vertex-language/vsc/build/buildcache"
	"github.com/vertex-language/vsc/timing"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	defer timing.Start("native bindings")()
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
	n, findKey, err := cachedFindNative(abs, folder.Native, target, c.main.minOS(target))
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

	vs, thunks, skipped, err := cachedBindings(n, findKey, pkgName, public)
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
	r := c.nativeObjectsAll([]string{dir})[0]
	return r.objs, r.need, r.err
}

// nativeObjectsAll is nativeObjects for several folders at once. Every
// C++ module they reach is compiled once, the modules concurrently, and
// each folder's objects come in the order nativeObjects gives them:
// the folder's own, then its imports', depth first.
func (c *common) nativeObjectsAll(dirs []string) []nativeResult {
	out := make([]nativeResult, len(dirs))
	abs := make([]string, len(dirs))
	var mods []string
	reached := map[string]bool{}
	var reach func(string)
	reach = func(d string) {
		if reached[d] {
			return
		}
		reached[d] = true
		nat := c.natives[d]
		if nat == nil {
			return
		}
		mods = append(mods, d)
		for _, dep := range nat.deps {
			reach(dep)
		}
	}
	for i, dir := range dirs {
		a, err := filepath.Abs(dir)
		if err != nil {
			out[i].err = err
			continue
		}
		abs[i] = a
		reach(a)
	}

	built := make(map[string]*nativeResult, len(mods))
	for _, d := range mods {
		built[d] = &nativeResult{}
	}
	var wg sync.WaitGroup
	for _, d := range mods {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nat, r := c.natives[d], built[d]
			doneNative := timing.Start("native C++ objects")
			r.objs, r.need, r.err = nat.n.Objects(nat.thunks, nativeWork())
			doneNative()
		}()
	}
	wg.Wait()

	for i := range dirs {
		if out[i].err != nil {
			continue
		}
		seen := map[string]bool{}
		var walk func(string) error
		walk = func(d string) error {
			if seen[d] {
				return nil
			}
			seen[d] = true
			r := built[d]
			if r == nil {
				return nil
			}
			if r.err != nil {
				return r.err
			}
			out[i].objs = append(out[i].objs, r.objs...)
			out[i].need = out[i].need.Merge(r.need)
			for _, dep := range c.natives[d].deps {
				if err := walk(dep); err != nil {
					return err
				}
			}
			return nil
		}
		if err := walk(abs[i]); err != nil {
			out[i] = nativeResult{need: out[i].need, err: err}
		}
	}
	return out
}

// The two halves of binding a folder's C++, through the build cache.
//
// Both run vcx over the module's interface and every header it includes
// -- the SDK's among them -- to learn what a build already learned last
// time: which module the folder is, and what Vertex declarations its
// exports are. The inputs are the folder's files, the interface units of
// the modules it imports, the SDK and the compiler, as for the native
// objects themselves (build.Native's cacheKey).

type foundNative struct {
	Module, Interface     string
	Imports               []string
	Libraries, Frameworks []string
}

func cachedFindNative(dir string, sources []string, target ir.Target, minOS string) (*build.Native, buildcache.Key, error) {
	sdk, _ := build.SDK()
	h := buildcache.New("native-find").String(target.String()).String(minOS).String(sdk).String(dir).Strings(sources)
	hashFiles(h, folderFiles(dir))
	key := h.Key()
	if data, ok := buildcache.Get(key); ok {
		var f foundNative
		if json.Unmarshal(data, &f) == nil {
			timing.Count("cache hit: native find", 1)
			return &build.Native{
				Dir: dir, Sources: sources, Target: target, MinOS: minOS,
				Module: f.Module, Interface: f.Interface, Imports: f.Imports,
				Libraries: f.Libraries, Frameworks: f.Frameworks,
			}, key, nil
		}
	}
	n, err := build.FindNative(dir, sources, target, minOS)
	if err != nil {
		return nil, key, err
	}
	if data, err := json.Marshal(foundNative{n.Module, n.Interface, n.Imports, n.Libraries, n.Frameworks}); err == nil {
		buildcache.Put(key, data)
	}
	return n, key, nil
}

type boundNative struct {
	VS, Thunks []byte
	Skipped    []string
}

func cachedBindings(n *build.Native, findKey buildcache.Key, pkgName string, public bool) ([]byte, []byte, []string, error) {
	h := buildcache.New("native-bind").Bytes(findKey[:]).String(pkgName).String(fmt.Sprint(public))
	var mods []string
	for name, file := range n.Modules {
		mods = append(mods, name+"="+file)
	}
	sort.Strings(mods)
	h.Strings(mods)
	var files []string
	for _, file := range n.Modules {
		files = append(files, file)
	}
	sort.Strings(files)
	hashFiles(h, files)
	key := h.Key()
	if data, ok := buildcache.Get(key); ok {
		var b boundNative
		if json.Unmarshal(data, &b) == nil {
			timing.Count("cache hit: native bindings", 1)
			return b.VS, b.Thunks, b.Skipped, nil
		}
	}
	vs, thunks, skipped, err := n.Bindings(pkgName, public)
	if err != nil {
		return nil, nil, nil, err
	}
	if data, err := json.Marshal(boundNative{vs, thunks, skipped}); err == nil {
		buildcache.Put(key, data)
	}
	return vs, thunks, skipped, nil
}

// folderFiles is every file directly in dir, sorted.
func folderFiles(dir string) []string {
	var files []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		// The folder's Vertex is no input to its C++: an edit to a .vs
		// file must not rebind and rebuild the module.
		if !e.IsDir() && filepath.Ext(e.Name()) != ".vs" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files
}

// hashFiles adds each file's name and contents to h; a file that cannot
// be read is added as missing.
func hashFiles(h *buildcache.Hasher, files []string) {
	h.String(fmt.Sprint(len(files)))
	for _, f := range files {
		h.String(f)
		data, err := os.ReadFile(f)
		if err != nil {
			h.String("<missing>")
			continue
		}
		h.Bytes(data)
	}
}
