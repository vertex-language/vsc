package importer

import (
	"os"
	"path/filepath"
	"strings"
)

// CacheEnv is the variable that says where fetched packages are kept.
const CacheEnv = "VERTEXCACHE"

// CacheDir returns the root directory where fetched packages are cached.
// It checks VERTEXCACHE, falling back to os.UserCacheDir() ("vertex" subfolder).
func CacheDir() string {
	if dir := os.Getenv(CacheEnv); dir != "" {
		return dir
	}
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "vertex")
	}
	// Fall back to a temporary directory if user cache is unavailable.
	return filepath.Join(os.TempDir(), "vertex-cache")
}

// PackageDir returns the cached checkout directory for a module and ref:
// <cache>/pkg/<repo>@<ref>.
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
