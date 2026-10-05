package importer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/importer"
)

// TestLookup tells the kinds of import path apart: the standard library's,
// a path that is already a repository, and a relative folder, which is
// nothing to do with fetching.
func TestLookup(t *testing.T) {
	cases := []struct {
		path   string
		want   string
		stdlib bool
		ok     bool
	}{
		{"net/tcp", "https://github.com/vertex-language/net", true, true},
		{"net/udp", "https://github.com/vertex-language/net", true, true},
		{"time", "https://github.com/vertex-language/time", true, true},
		{"ui/window", "https://github.com/vertex-language/ui", true, true},
		{"github.com/you/thing", "https://github.com/you/thing", false, true},
		{"github.com/you/thing/text", "https://github.com/you/thing", false, true},
		{"example.org/a/b", "https://example.org/a/b", false, true},
		// Any path naming no host is the standard library's.
		{"util/text", "https://github.com/vertex-language/util", true, true},
		{"tcp", "https://github.com/vertex-language/tcp", true, true},
		// Not a repository: a host and nothing else.
		{"github.com/you", "", false, false},
		{"", "", false, false},
		{"./util", "", false, false},
	}
	for _, c := range cases {
		m, ok := importer.Lookup(c.path)
		if ok != c.ok {
			t.Errorf("Lookup(%q) ok = %v, want %v", c.path, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if m.Repo != c.want {
			t.Errorf("Lookup(%q) repo = %q, want %q", c.path, m.Repo, c.want)
		}
		if m.Stdlib != c.stdlib {
			t.Errorf("Lookup(%q) stdlib = %v, want %v", c.path, m.Stdlib, c.stdlib)
		}
	}
	if !importer.IsStdlib("net/tcp") || !importer.IsStdlib("time") {
		t.Error("net/tcp and time are the standard library's")
	}
	if importer.IsStdlib("github.com/you/thing") || importer.IsStdlib("./util") {
		t.Error("a repository path and a relative one are not the standard library's")
	}
}

// TestLookupRef reads a branch or tag off the path, so that two programs
// may want different ones.
func TestLookupRef(t *testing.T) {
	m, ok := importer.Lookup("github.com/you/thing@v2")
	if !ok || m.Ref != "v2" || m.Repo != "https://github.com/you/thing" {
		t.Fatalf("Lookup = %+v, %v", m, ok)
	}
	a := importer.PackageDir(m, "/c")
	b := importer.PackageDir(importer.Module{Repo: m.Repo}, "/c")
	if a == b {
		t.Errorf("two refs share one directory: %s", a)
	}
}

// TestPackageDir keeps a checkout under the repository's own name, so
// that what is in the cache can be read at a glance.
func TestPackageDir(t *testing.T) {
	m, _ := importer.Lookup("net/tcp")
	dir := importer.PackageDir(m, "/cache")
	want := filepath.Join("/cache", "pkg", "github.com", "vertex-language", "net@default")
	if dir != want {
		t.Errorf("PackageDir = %q, want %q", dir, want)
	}
}

// TestCacheDirFromEnv lets the cache be put somewhere else, which a build
// that must not touch the real one relies on.
func TestCacheDirFromEnv(t *testing.T) {
	t.Setenv(importer.CacheEnv, "/somewhere/else")
	if got := importer.CacheDir(); got != "/somewhere/else" {
		t.Errorf("CacheDir = %q", got)
	}
}

// TestOfflineWithoutCache fails rather than reaching the network, and
// says where the package was expected.
func TestOfflineWithoutCache(t *testing.T) {
	m, _ := importer.Lookup("net/tcp")
	_, err := importer.Fetch(m, importer.Options{Cache: t.TempDir(), Offline: true})
	if err == nil {
		t.Fatal("an offline fetch of a package not in the cache succeeded")
	}
	if !strings.Contains(err.Error(), "net/tcp") {
		t.Errorf("error does not name the package: %v", err)
	}
}

// TestResolveUnknownPath says what a path that names nothing may be,
// rather than reporting a failed download.
func TestResolveUnknownPath(t *testing.T) {
	_, err := importer.Resolve("github.com/you", importer.Options{Cache: t.TempDir(), Offline: true})
	if err == nil {
		t.Fatal("a path that is not a package resolved")
	}
	if !strings.Contains(err.Error(), "net/tcp") {
		t.Errorf("error does not list the reserved names: %v", err)
	}
}

// TestResolveLocalCheckout finds the source a standard library path names
// in a checkout already in the cache: the folder the rest of the path is.
//
// It uses a checkout written here rather than a fetched one, so it is the
// resolution that is under test and not the network.
func TestResolveLocalCheckout(t *testing.T) {
	cache := t.TempDir()
	m, _ := importer.Lookup("net/tcp")
	root := importer.PackageDir(m, cache)
	writeFiles(t, root, map[string]string{
		".git/HEAD":     "ref: refs/heads/main\n",
		"tcp/stream.vs": "package tcp\n",
	})

	dir, err := importer.Resolve("net/tcp", importer.Options{Cache: cache, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "tcp"); filepath.Clean(dir) != want {
		t.Errorf("Resolve = %q, want %q", dir, want)
	}
}

// TestResolveMissingFolder names the folder it looked for: a path is a
// folder, and nothing else can answer for it.
func TestResolveMissingFolder(t *testing.T) {
	cache := t.TempDir()
	m, _ := importer.Lookup("net/tcp")
	root := importer.PackageDir(m, cache)
	writeFiles(t, root, map[string]string{
		".git/HEAD":     "ref: refs/heads/main\n",
		"udp/socket.vs": "package udp\n",
	})

	_, err := importer.Resolve("net/tcp", importer.Options{Cache: cache, Offline: true})
	if err == nil {
		t.Fatal("a checkout with no tcp folder resolved net/tcp")
	}
	if !strings.Contains(err.Error(), filepath.Join(root, "tcp")) {
		t.Errorf("error does not name the folder: %v", err)
	}
}

// TestResolveRepositoryRoot: a path that is the repository is its root.
func TestResolveRepositoryRoot(t *testing.T) {
	cache := t.TempDir()
	m, _ := importer.Lookup("time")
	root := importer.PackageDir(m, cache)
	writeFiles(t, root, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		"time.vs":   "package time\n",
	})

	dir, err := importer.Resolve("time", importer.Options{Cache: cache, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Clean(root); filepath.Clean(dir) != want {
		t.Errorf("Resolve = %q, want %q", dir, want)
	}
}

// TestResolveSubfolders: each folder of a repository is its own package.
func TestResolveSubfolders(t *testing.T) {
	cache := t.TempDir()
	m, _ := importer.Lookup("crypto/sha256")
	root := importer.PackageDir(m, cache)
	writeFiles(t, root, map[string]string{
		".git/HEAD":        "ref: refs/heads/main\n",
		"sha256/sha256.vs": "package sha256\n",
		"sha512/sha512.vs": "package sha512\n",
	})

	dir, err := importer.Resolve("crypto/sha256", importer.Options{Cache: cache, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "sha256"); filepath.Clean(dir) != want {
		t.Errorf("Resolve = %q, want %q", dir, want)
	}
}

// TestResolveCxxFolder: a folder of C++ alone is a package, whose module
// is its API.
func TestResolveCxxFolder(t *testing.T) {
	cache := t.TempDir()
	m, _ := importer.Lookup("math")
	root := importer.PackageDir(m, cache)
	writeFiles(t, root, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		"math.cpp":  "export module math;\n",
	})
	if _, err := importer.Resolve("math", importer.Options{Cache: cache, Offline: true}); err != nil {
		t.Fatal(err)
	}
}

// TestSums: a hash recorded once is held to after, and is Go's h1 form.
func TestSums(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.vs": "package a\n", ".git/HEAD": "x\n"})
	h, err := importer.Hash(dir)
	if err != nil || !strings.HasPrefix(h, "h1:") {
		t.Fatalf("Hash = %q, %v", h, err)
	}
	writeFiles(t, dir, map[string]string{".git/HEAD": "changed\n"})
	if again, _ := importer.Hash(dir); again != h {
		t.Errorf(".git is part of the hash: %s then %s", h, again)
	}
	path := filepath.Join(t.TempDir(), "vs.sum")
	sums, _ := importer.ReadSums(path)
	if err := sums.Check("net", "v1.0.0", h); err != nil {
		t.Fatal(err)
	}
	if err := sums.Save(); err != nil {
		t.Fatal(err)
	}
	sums, _ = importer.ReadSums(path)
	if err := sums.Check("net", "v1.0.0", "h1:other"); err == nil {
		t.Error("a changed hash was accepted")
	}
	if got := importer.ModuleOf("net/tcp"); got != "net" {
		t.Errorf("ModuleOf(net/tcp) = %q", got)
	}
	if got := importer.ModuleOf("github.com/you/thing/x/y"); got != "github.com/you/thing" {
		t.Errorf("ModuleOf = %q", got)
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
