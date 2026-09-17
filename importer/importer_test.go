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

// TestResolveLocalCheckout finds the source a reserved path names by
// reading the package's manifest: the library product with that name
// says which target it is, and the target says where the source is.
//
// It uses a package written here rather than a fetched one, so it is the
// manifest reading that is under test and not the network.
func TestResolveLocalCheckout(t *testing.T) {
	cache := t.TempDir()
	m, _ := importer.Lookup("net/tcp")
	root := importer.PackageDir(m, cache)
	writeFiles(t, root, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		"package.vs": `import PackageDescription
let package = Package(
    name: "net",
    products: [
        .library(name: "net/tcp", targets: ["tcp"]),
    ],
    targets: [
        .target(name: "tcp", path: "tcp"),
    ]
)
`,
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

// TestResolveWrongProduct names what the package does offer, since a
// package whose library is called something else is the likely mistake.
func TestResolveWrongProduct(t *testing.T) {
	cache := t.TempDir()
	m, _ := importer.Lookup("net/tcp")
	root := importer.PackageDir(m, cache)
	writeFiles(t, root, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		"package.vs": `import PackageDescription
let package = Package(
    name: "net",
    products: [
        .library(name: "net/udp", targets: ["udp"]),
    ],
    targets: [
        .target(name: "udp", path: "udp"),
    ]
)
`,
		"udp/socket.vs": "package udp\n",
	})

	_, err := importer.Resolve("net/tcp", importer.Options{Cache: cache, Offline: true})
	if err == nil {
		t.Fatal("a package offering no such library resolved")
	}
	if !strings.Contains(err.Error(), "net/udp") {
		t.Errorf("error does not say what the package offers: %v", err)
	}
}

// TestResolvePureFolderWithoutManifest verifies that a pure Vertex repository
// without package.vs resolves directly to its root when it has .vs source files.
func TestResolvePureFolderWithoutManifest(t *testing.T) {
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

// TestResolvePureSubfolderWithoutManifest verifies that subpackages in a pure
// Vertex repo without package.vs (Golang style) resolve to their respective subfolders.
func TestResolvePureSubfolderWithoutManifest(t *testing.T) {
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

// TestResolvePureSubfolderBesideManifest verifies that a pure .vs folder inside a repo
// that has a package.vs (for C targets) still resolves as a package without needing to be
// listed in package.vs.
func TestResolvePureSubfolderBesideManifest(t *testing.T) {
	cache := t.TempDir()
	m, _ := importer.Lookup("net/ip")
	root := importer.PackageDir(m, cache)
	writeFiles(t, root, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		"package.vs": `import PackageDescription
let package = Package(
    name: "net",
    products: [
        .library(name: "net/tcp", targets: ["tcp"]),
    ],
    targets: [
        .target(name: "tcp", path: "tcp"),
    ]
)
`,
		"tcp/stream.vs": "package tcp\n",
		"ip/ip.vs":      "package ip\n",
	})

	dir, err := importer.Resolve("net/ip", importer.Options{Cache: cache, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "ip"); filepath.Clean(dir) != want {
		t.Errorf("Resolve = %q, want %q", dir, want)
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
