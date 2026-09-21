package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A Language is what a source file is written in.
type Language int

const (
	Swift Language = iota
	C
	CXX
	ObjC
	ObjCXX
	Assembly
)

// languageOf is a source file's language by its extension, and false for a
// file that is not a source: a header, a module map, a resource.
func languageOf(path string) (Language, bool) {
	switch filepath.Ext(path) {
	case ".swift", ".vs":
		return Swift, true
	case ".c":
		return C, true
	case ".cpp", ".cc", ".cxx", ".c++":
		return CXX, true
	case ".m":
		return ObjC, true
	case ".mm":
		return ObjCXX, true
	case ".s", ".S":
		return Assembly, true
	}
	return 0, false
}

// A SourceFile is one file of a target.
type SourceFile struct {
	Path     string
	Language Language
}

// A Package is a manifest laid out on disk.
type Package struct {
	Root     string
	Manifest *Manifest
	// Targets are in build order: every target after the targets it
	// depends on.
	Targets []*ResolvedTarget
	byName  map[string]*ResolvedTarget
}

// A ResolvedTarget is a target with its directory and files found.
type ResolvedTarget struct {
	*Target
	Dir     string
	Sources []SourceFile
	// PublicHeaders is a C-family target's public header directory.
	PublicHeaders string
	// HeaderSearchPaths are target header search paths made absolute.
	HeaderSearchPaths []string
	// Defines maps tool ("c", "cxx", "swift") to preprocessor defines.
	Defines      map[string][]string
	UnsafeFlags  map[string][]string // by tool, and "linker"
	Libraries    []string
	Frameworks   []string
	CxxInterop   bool
	Dependencies []*ResolvedTarget
}

// Swift reports whether the target is written in Swift.
func (t *ResolvedTarget) Swift() bool {
	return len(t.Sources) > 0 && t.Sources[0].Language == Swift
}

// UsesCXX reports whether any of the target's files are C++.
func (t *ResolvedTarget) UsesCXX() bool {
	for _, s := range t.Sources {
		if s.Language == CXX || s.Language == ObjCXX {
			return true
		}
	}
	return false
}

// Closure is the target and everything it depends on, in build order.
func (t *ResolvedTarget) Closure() []*ResolvedTarget {
	var out []*ResolvedTarget
	seen := map[*ResolvedTarget]bool{}
	var visit func(*ResolvedTarget)
	visit = func(n *ResolvedTarget) {
		if seen[n] {
			return
		}
		seen[n] = true
		for _, d := range n.Dependencies {
			visit(d)
		}
		out = append(out, n)
	}
	visit(t)
	return out
}

// IncludeDirs returns all header include directories for compiling a C-family target.
func (t *ResolvedTarget) IncludeDirs() []string {
	var out []string
	if t.PublicHeaders != "" {
		out = append(out, t.PublicHeaders)
	}
	out = append(out, t.HeaderSearchPaths...)
	for _, d := range t.Closure() {
		if d != t && d.PublicHeaders != "" {
			out = append(out, d.PublicHeaders)
		}
	}
	return out
}

// Target is the resolved target of a name.
func (p *Package) Target(name string) *ResolvedTarget { return p.byName[name] }

// Executable represents an executable product or target built by the package.
type Executable struct {
	Name   string
	Target *ResolvedTarget
}

// Executables are the programs, by product name.
func (p *Package) Executables() []Executable {
	var out []Executable
	named := map[string]bool{}
	for _, prod := range p.Manifest.Products {
		if prod.Kind != "executable" || len(prod.Targets) == 0 {
			continue
		}
		if t := p.byName[prod.Targets[0]]; t != nil {
			out = append(out, Executable{Name: prod.Name, Target: t})
			named[t.Name] = true
		}
	}
	for _, t := range p.Targets {
		if t.Kind == TargetExecutable && !named[t.Name] {
			out = append(out, Executable{Name: t.Name, Target: t})
		}
	}
	return out
}

// The directories SwiftPM looks for targets in, in order.
var (
	sourceRoots = []string{"Sources", "Source", "src", "srcs"}
	testRoots   = []string{"Tests", "Sources", "Source", "src", "srcs"}
)

// Resolve resolves package targets, dependencies, and build order for a given platform and configuration.
func Resolve(root string, m *Manifest, platform, config string) (*Package, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	p := &Package{Root: root, Manifest: m, byName: map[string]*ResolvedTarget{}}
	// A target whose directory is inside another's owns that directory:
	// the outer target's files stop where the inner one's begin.
	var owned []string
	for i := range m.Targets {
		if path := m.Targets[i].Path; path != nil {
			owned = append(owned, filepath.Clean(filepath.Join(root, *path)))
		}
	}
	var all []*ResolvedTarget
	for i := range m.Targets {
		t := &m.Targets[i]
		if _, dup := p.byName[t.Name]; dup {
			return nil, fmt.Errorf("duplicate target named '%s'", t.Name)
		}
		switch t.Kind {
		case TargetRegular, TargetExecutable, TargetTest:
		default:
			return nil, fmt.Errorf("target '%s' is a %s target, which vsc does not build yet", t.Name, t.Kind)
		}
		rt, err := resolveTarget(root, t, platform, config, owned)
		if err != nil {
			return nil, err
		}
		p.byName[t.Name] = rt
		all = append(all, rt)
	}
	for _, rt := range all {
		for _, d := range rt.Target.Dependencies {
			if d.Condition != nil && !d.Condition.holds(platform, config) {
				continue
			}
			dep, ok := p.byName[d.Name]
			switch {
			case d.Kind == "product" && d.Package != "":
				return nil, fmt.Errorf("target '%s' depends on product '%s' of package '%s': dependencies on other packages are not built yet",
					rt.Name, d.Name, d.Package)
			case !ok:
				return nil, fmt.Errorf("target '%s' depends on '%s', which is not a target of package '%s'", rt.Name, d.Name, m.Name)
			}
			rt.Dependencies = append(rt.Dependencies, dep)
		}
	}
	order, err := buildOrder(all)
	if err != nil {
		return nil, err
	}
	p.Targets = order
	return p, nil
}

func resolveTarget(root string, t *Target, platform, config string, owned []string) (*ResolvedTarget, error) {
	rt := &ResolvedTarget{
		Target:      t,
		Defines:     map[string][]string{},
		UnsafeFlags: map[string][]string{},
	}
	if t.Path != nil {
		rt.Dir = filepath.Join(root, *t.Path)
	} else {
		roots := sourceRoots
		if t.Kind == TargetTest {
			roots = testRoots
		}
		for _, r := range roots {
			dir := filepath.Join(root, r, t.Name)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				rt.Dir = dir
				break
			}
		}
		if rt.Dir == "" {
			return nil, fmt.Errorf("could not find source files for target '%s': expected them in '%s'",
				t.Name, filepath.Join(roots[0], t.Name))
		}
	}
	if info, err := os.Stat(rt.Dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("target '%s': '%s' is not a directory", t.Name, rt.Dir)
	}

	files, err := targetFiles(rt.Dir, t, owned)
	if err != nil {
		return nil, err
	}
	swift, cfamily, elsewhere := 0, 0, 0
	for _, f := range files {
		lang, ok := languageOf(f)
		if !ok {
			continue
		}
		if !ForPlatform(f, platform) {
			elsewhere++
			continue
		}
		rt.Sources = append(rt.Sources, SourceFile{Path: f, Language: lang})
		if lang == Swift {
			swift++
		} else {
			cfamily++
		}
	}
	switch {
	case len(rt.Sources) == 0 && elsewhere > 0:
		// Every source is another platform's: on this one the target is
		// empty, and a program that needs nothing from it still builds.
	case len(rt.Sources) == 0:
		return nil, fmt.Errorf("target '%s' has no sources in '%s'", t.Name, rt.Dir)
	case swift > 0 && cfamily > 0:
		return nil, fmt.Errorf("target '%s' contains mixed language source files: Swift and C-family sources go in separate targets", t.Name)
	}

	if cfamily > 0 || elsewhere > 0 {
		headers := "include"
		if t.PublicHeadersPath != nil {
			headers = *t.PublicHeadersPath
		}
		dir := filepath.Join(rt.Dir, headers)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			rt.PublicHeaders = dir
		}
	}

	for _, s := range t.Settings {
		if s.Condition != nil && !s.Condition.holds(platform, config) {
			continue
		}
		switch s.Kind {
		case "define":
			rt.Defines[s.Tool] = append(rt.Defines[s.Tool], s.Values...)
		case "headerSearchPath":
			for _, v := range s.Values {
				rt.HeaderSearchPaths = append(rt.HeaderSearchPaths, filepath.Join(rt.Dir, v))
			}
		case "unsafeFlags":
			rt.UnsafeFlags[s.Tool] = append(rt.UnsafeFlags[s.Tool], s.Values...)
		case "linkedLibrary":
			rt.Libraries = append(rt.Libraries, s.Values...)
		case "linkedFramework":
			rt.Frameworks = append(rt.Frameworks, s.Values...)
		case "interoperabilityMode":
			rt.CxxInterop = len(s.Values) > 0 && s.Values[0] == "Cxx"
		}
	}
	return rt, nil
}

// targetFiles is every file under a target's directory that the target
// includes: not hidden, not excluded, and under one of `sources:` where the
// manifest listed them.
func targetFiles(dir string, t *Target, owned []string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Another target's directory, inside this one's: its files are
		// that target's.
		if d.IsDir() && path != dir {
			clean := filepath.Clean(path)
			for _, o := range owned {
				if clean == o {
					return filepath.SkipDir
				}
			}
		}
		rel, _ := filepath.Rel(dir, path)
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel != "." && underAny(rel, t.Exclude) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if t.HasSources && !underAny(rel, t.Sources) {
			return nil
		}
		out = append(out, path)
		return nil
	})
	sort.Strings(out)
	return out, err
}

// underAny reports whether rel is one of paths or inside one of them.
func underAny(rel string, paths []string) bool {
	rel = filepath.ToSlash(rel)
	for _, p := range paths {
		p = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(p)), "/")
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func (c *Condition) holds(platform, config string) bool {
	if len(c.Platforms) > 0 {
		found := false
		for _, p := range c.Platforms {
			if p == platform {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return c.Config == "" || c.Config == config
}

// buildOrder puts every target after its dependencies, in the manifest's
// order otherwise, and reports a cycle the way SwiftPM does.
func buildOrder(targets []*ResolvedTarget) ([]*ResolvedTarget, error) {
	const (
		unseen = iota
		visiting
		done
	)
	state := map[*ResolvedTarget]int{}
	var out []*ResolvedTarget
	var path []string
	var visit func(*ResolvedTarget) error
	visit = func(t *ResolvedTarget) error {
		switch state[t] {
		case done:
			return nil
		case visiting:
			cycle := append(append([]string{}, path...), t.Name)
			for i, n := range cycle {
				if n == t.Name {
					cycle = cycle[i:]
					break
				}
			}
			return fmt.Errorf("cyclic dependency declaration found: %s", strings.Join(cycle, " -> "))
		}
		state[t] = visiting
		path = append(path, t.Name)
		for _, d := range t.Dependencies {
			if err := visit(d); err != nil {
				return err
			}
		}
		path = path[:len(path)-1]
		state[t] = done
		out = append(out, t)
		return nil
	}
	for _, t := range targets {
		if err := visit(t); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// sourcePlatforms are the file-name suffixes that limit a source to some
// platforms, as Go's do: cwindow_darwin.m is Cocoa's and cwindow_android.c
// NativeActivity's, and each target builds the one that is its own.
var sourcePlatforms = map[string][]string{
	"darwin":  {"macos", "ios", "tvos", "watchos", "visionos", "maccatalyst"},
	"macos":   {"macos"},
	"ios":     {"ios"},
	"windows": {"windows"},
	"linux":   {"linux"},
	"android": {"android"},
}

// ForPlatform reports whether the source file at path is built for
// platform: yes, unless its base name ends in a platform suffix
// (name_darwin.m) that does not include it.
func ForPlatform(path, platform string) bool {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	i := strings.LastIndexByte(base, '_')
	if i < 0 {
		return true
	}
	plats, ok := sourcePlatforms[base[i+1:]]
	if !ok {
		return true
	}
	for _, p := range plats {
		if p == platform {
			return true
		}
	}
	return false
}

// PlatformOf is the manifest platform name for an IR target's use path:
// "aarch64/android" is "android".
func PlatformOf(use string) string {
	switch {
	case strings.HasSuffix(use, "/windows"):
		return "windows"
	case strings.HasSuffix(use, "/android"):
		return "android"
	case strings.HasSuffix(use, "/linux"):
		return "linux"
	}
	return "macos"
}
