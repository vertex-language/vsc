package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve is the directory of source an import path names, fetching the
// repository it is in if this machine does not have it yet.
//
// The directory is the package: its .vs files, and the C++ module beside
// them. Nothing is compiled here.
func Resolve(path string, opts Options) (string, error) {
	m, ok := Lookup(path)
	if !ok {
		return "", fmt.Errorf("'%s' is not a package this compiler knows how to fetch: "+
			"a standard library package (like %s), or a repository like github.com/you/thing",
			path, strings.Join(StdlibExamples(), ", "))
	}
	if m.Ref == "" {
		m.Ref = opts.Ref
	}
	root, err := Fetch(m, opts)
	if err != nil {
		return "", err
	}
	return sourceDir(m, root)
}

// Checkout is the source directory for path in a checkout already on disk
// -- one named by -replace, a vs.mod replace, or a vs.work -- where sub is
// the folder of it the path names ("" for the checkout itself).
func Checkout(path, root, sub string) (string, error) {
	m, ok := Lookup(path)
	if !ok {
		m = Module{Path: path}
	}
	return sourceDirIn(m, root, sub)
}

// sourceDir is the directory inside a checkout that an import path names:
// the folder the rest of the path is, as in Go.
func sourceDir(m Module, root string) (string, error) {
	return sourceDirIn(m, root, m.subdir())
}

// sourceDirIn is sourceDir with the folder inside the checkout already
// worked out: "" for the checkout's root.
func sourceDirIn(m Module, root, sub string) (string, error) {
	dir := root
	if sub != "" {
		dir = filepath.Join(root, filepath.FromSlash(sub))
	}
	if err := hasSource(dir); err != nil {
		return "", fmt.Errorf("package '%s': %w", m.Path, err)
	}
	return dir, nil
}

// hasSource reports whether a directory holds a package: Vertex source, or
// a C++ module.
func hasSource(dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("no directory %s", dir)
	}
	for _, pattern := range []string{"*.vs", "*.cpp", "*.cppm", "*.mm"} {
		if found, err := filepath.Glob(filepath.Join(dir, pattern)); err == nil && len(found) > 0 {
			return nil
		}
	}
	return fmt.Errorf("%s holds no source", dir)
}
