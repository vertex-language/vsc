package importer

import (
	"os"
	"path/filepath"
	"strings"
)

// CacheEnv is the variable that says where fetched packages are kept.
const CacheEnv = "VERTEXCACHE"

// CacheDir is where fetched packages are kept: VERTEXCACHE where it is
// set, and a "vertex" directory in this machine's cache otherwise --
// ~/Library/Caches on macOS, ~/.cache on Linux, AppData on Windows.
//
// Everything under it is fetched and can be deleted: a build that finds
// it empty does the same work again over the network.
func CacheDir() string {
	if dir := os.Getenv(CacheEnv); dir != "" {
		return dir
	}
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "vertex")
	}
	// No home directory to speak of. A directory beside the program's
	// own work is better than failing the build.
	return filepath.Join(os.TempDir(), "vertex-cache")
}

// PackageDir is where a module's checkout belongs, whether or not it is
// there yet: the repository's own name, and the ref it was taken at.
//
//	<cache>/pkg/github.com/vertex-language/net-tcp@main
//
// The ref is part of the name so that two programs wanting different
// branches of one repository do not fight over a single directory.
func PackageDir(m Module, cache string) string {
	name := strings.TrimPrefix(m.Repo, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.TrimSuffix(name, ".git")
	if ref := m.Ref; ref != "" {
		name += "@" + escapeRef(ref)
	} else {
		name += "@" + defaultRefName
	}
	return filepath.Join(cache, "pkg", filepath.FromSlash(name))
}

// defaultRefName stands for "whatever the repository's default branch
// is", in a directory name, since that is not known before it is cloned.
const defaultRefName = "default"

// escapeRef makes a ref safe as one path segment: a branch may hold a
// slash, and nothing else in a name here may.
func escapeRef(ref string) string {
	return strings.NewReplacer("/", "-", string(filepath.Separator), "-").Replace(ref)
}
