package importer_test

import (
	"path/filepath"
	"testing"

	"github.com/vertex-language/vsc/importer"
)

func gitConfig(url string) string {
	return "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + url + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
}

// TestEnclosingPath reads a checkout's import path from its vs.mod, then
// where it would be fetched from, then its folder's name.
func TestEnclosingPath(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"the standard library's organization is left off",
			map[string]string{".git/config": gitConfig("https://github.com/vertex-language/crypto")},
			"crypto"},
		{"a URL ending in .git is the same repository",
			map[string]string{".git/config": gitConfig("https://github.com/vertex-language/net.git")},
			"net"},
		{"any other repository is its whole path",
			map[string]string{".git/config": gitConfig("git@github.com:you/thing.git")},
			"github.com/you/thing"},
		{"a vs.mod's module line comes first",
			map[string]string{".git/config": gitConfig("https://github.com/someone/fork"),
				"vs.mod": "module github.com/vertex-language/time\n"},
			"time"},
		{"with no remote, the vs.mod",
			map[string]string{".git/HEAD": "ref: refs/heads/main\n",
				"vs.mod": "module github.com/you/clock\n"},
			"github.com/you/clock"},
		{"with neither, the folder's name",
			map[string]string{".git/HEAD": "ref: refs/heads/main\n"},
			"repo"},
	} {
		root := filepath.Join(t.TempDir(), "repo")
		writeFiles(t, root, c.files)
		writeFiles(t, root, map[string]string{"tests/all/main.vs": "package main\n"})
		r, ok := importer.Enclosing(filepath.Join(root, "tests", "all"))
		if !ok || r.Dir != root || r.Path != c.want {
			t.Errorf("%s: Enclosing = %+v, %v; want %s at %s", c.name, r, ok, c.want, root)
		}
	}
}

// TestLocalIsTheCheckoutItself: a file in a checkout imports that
// checkout's packages from disk, and anything else is not its answer.
func TestLocalIsTheCheckoutItself(t *testing.T) {
	root := filepath.Join(t.TempDir(), "crypto")
	writeFiles(t, root, map[string]string{
		".git/config":       gitConfig("https://github.com/vertex-language/crypto"),
		"sha256/sha256.vs":  "package sha256\n",
		"hmac/hmac.vs":      "package hmac\n",
		"tests/all/main.vs": "package main\n",
	})
	from := filepath.Join(root, "tests", "all")
	for path, want := range map[string]string{
		"crypto/sha256": filepath.Join(root, "sha256"),
		"crypto/hmac":   filepath.Join(root, "hmac"),
		"encoding/hex":  "",
		"cryptography":  "",
		"./sha256":      "",
	} {
		got, err := importer.Local(path, from)
		if err != nil || got != want {
			t.Errorf("Local(%q) = %q, %v; want %q", path, got, err, want)
		}
	}
}

// TestCheckoutOfAReplacement: -replace names either the checkout or the
// folder itself, and the folder is what is left of the path after the name.
func TestCheckoutOfAReplacement(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"tcp/stream.vs": "package tcp\n"})
	if got, err := importer.Checkout("net/tcp", root, "tcp"); err != nil || got != filepath.Join(root, "tcp") {
		t.Errorf("net=root: %q, %v", got, err)
	}
	if got, err := importer.Checkout("net/tcp", filepath.Join(root, "tcp"), ""); err != nil || got != filepath.Join(root, "tcp") {
		t.Errorf("net/tcp=root/tcp: %q, %v", got, err)
	}
}
