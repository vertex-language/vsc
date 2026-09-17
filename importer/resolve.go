package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/vsc/pkg"
	"github.com/vertex-language/vsc/token"
)

// Resolve is the directory of source an import path names, fetching the
// package it is in if this machine does not have it yet.
//
// The directory is one a build already knows what to do with: the .vs
// files in it are the module, and the manifest above it says how to build
// the C-family targets they depend on. Nothing is compiled here.
func Resolve(path string, opts Options) (string, error) {
	m, ok := Lookup(path)
	if !ok {
		return "", fmt.Errorf("'%s' is not a package this compiler knows how to fetch: "+
			"a standard library package (like %s), or a repository like github.com/you/thing",
			path, strings.Join(StdlibExamples(), ", "))
	}
	root, err := Fetch(m, opts)
	if err != nil {
		return "", err
	}
	return sourceDir(m, root)
}

// Checkout is the source directory for path in a checkout already on disk
// -- one named by -replace -- where sub is the folder of it the path names
// ("" for the checkout itself). A manifest's library products are asked
// first; without one, or where none is the path, the folder is the answer.
func Checkout(path, root, sub string) (string, error) {
	m, ok := Lookup(path)
	if !ok {
		m = Module{Path: path}
	}
	return sourceDirIn(m, root, sub)
}

// sourceDir is the directory inside a checkout that an import path names.
//
// A path that carries its own directory -- github.com/you/thing/text --
// says where to look, and so does a standard library path. Where the
// checkout has a manifest, its library product of that name is asked first,
// since its target may be anywhere: time's is time/time.
func sourceDir(m Module, root string) (string, error) {
	return sourceDirIn(m, root, m.subdir())
}

// sourceDirIn is sourceDir with the folder inside the checkout already
// worked out: "" for the checkout's root.
func sourceDirIn(m Module, root, sub string) (string, error) {
	var manifestErr error
	if _, ok := pkg.FindManifest(root); ok {
		man, diags, err := pkg.Load(root)
		if err != nil {
			return "", fmt.Errorf("package '%s': %w", m.Path, err)
		}
		for _, d := range diags {
			if d.Severity == token.Error {
				return "", fmt.Errorf("package '%s': its manifest does not read: %s", m.Path, d.Message)
			}
		}

		target, err := productTarget(man, m.Path)
		if err == nil {
			p, err := pkg.Resolve(root, man, hostPlatform(), "debug")
			if err != nil {
				return "", fmt.Errorf("package '%s': %w", m.Path, err)
			}
			for _, t := range p.Targets {
				if t.Name == target {
					return t.Dir, nil
				}
			}
			return "", fmt.Errorf("package '%s': its manifest offers '%s' but declares no target '%s'",
				m.Path, m.Path, target)
		}
		manifestErr = err
	}

	if sub != "" {
		dir := filepath.Join(root, filepath.FromSlash(sub))
		if err := hasSource(dir); err == nil {
			return dir, nil
		}
	}

	if sub == "" {
		if err := hasSource(root); err == nil {
			return root, nil
		}
	}

	if manifestErr != nil {
		where := m.Repo
		if where == "" {
			where = root
		}
		return "", fmt.Errorf("package '%s' at %s: %w", m.Path, where, manifestErr)
	}

	if sub != "" {
		dir := filepath.Join(root, filepath.FromSlash(sub))
		return "", fmt.Errorf("package '%s': %w", m.Path, hasSource(dir))
	}
	return "", fmt.Errorf("package '%s': %w", m.Path, hasSource(root))
}

// productTarget is the target behind the library product named path.
func productTarget(man *pkg.Manifest, path string) (string, error) {
	var names []string
	for _, prod := range man.Products {
		if prod.Kind != "library" {
			continue
		}
		names = append(names, prod.Name)
		if prod.Name != path {
			continue
		}
		if len(prod.Targets) == 0 {
			return "", fmt.Errorf("its '%s' library names no target", path)
		}
		// A library of several targets is imported by its first, which
		// is the one the others are there for.
		return prod.Targets[0], nil
	}
	if len(names) == 0 {
		return "", fmt.Errorf("it offers no libraries")
	}
	return "", fmt.Errorf("it offers no library called '%s' (it has: %s)",
		path, strings.Join(names, ", "))
}

// hasSource reports whether a directory holds Vertex source.
func hasSource(dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("no directory %s in it", filepath.Base(dir))
	}
	found, err := filepath.Glob(filepath.Join(dir, "*.vs"))
	if err == nil && len(found) > 0 {
		return nil
	}
	found, err = filepath.Glob(filepath.Join(dir, "*.swift"))
	if err == nil && len(found) > 0 {
		return nil
	}
	return fmt.Errorf("%s holds no source", dir)
}

// hostPlatform is the platform name a manifest's conditions are read for.
func hostPlatform() string {
	if isWindows() {
		return "windows"
	}
	return "macos"
}
