package vsc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc"
)

// A fakePackages stands in for the fetching resolver, so that what is
// under test is which paths the compiler hands it and which it answers
// from disk -- not the network.
type fakePackages struct {
	local map[string]string
	dirs  map[string]string
	errs  map[string]error
	asked []string // what Fetch was asked for
	near  []string // what Local was asked for
}

func (p *fakePackages) Local(path, fromDir string) (string, error) {
	p.near = append(p.near, path)
	return p.local[path], nil
}

func (p *fakePackages) Fetch(path string) (string, error) {
	p.asked = append(p.asked, path)
	if err := p.errs[path]; err != nil {
		return "", err
	}
	return p.dirs[path], nil
}

// writePackage writes a folder of source and returns where it is.
func writePackage(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package " + name + "\n" + body
	if err := os.WriteFile(filepath.Join(dir, name+".vs"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestLocalAnswerComesFirst: the resolver's local answer -- a -replace, or
// the checkout the importing file is in -- beats a folder under a search
// root and a fetch, so a package's own tests build against it as it is.
func TestLocalAnswerComesFirst(t *testing.T) {
	root := t.TempDir()
	// A folder the search root would find, answering a different number, so
	// which one was compiled is visible in the program.
	writePackage(t, filepath.Join(root, "net", "tcp"), "tcp", "public func answer() -> Int32 { return 1 }\n")
	checkout := writePackage(t, filepath.Join(t.TempDir(), "tcp"), "tcp", "public func answer() -> Int32 { return 42 }\n")

	p := &fakePackages{
		local: map[string]string{"net/tcp": checkout},
		dirs:  map[string]string{"net/tcp": "/should/not/be/used"},
	}
	u, diags := compile(t, `
import "net/tcp"
func main() -> Int32 { return tcp.answer() }
`, vsc.Options{PackagePaths: []string{root}, Packages: p})
	for _, d := range diags {
		t.Fatalf("compile: %v", d)
	}
	if len(p.asked) != 0 {
		t.Errorf("resolver was asked to fetch %v, though it had the package locally", p.asked)
	}
	if len(u.Packages) != 1 || u.Packages[0].Dir != checkout {
		t.Fatalf("compiled %+v, want the local %s", u.Packages, checkout)
	}
}

// TestFolderBeatsFetchingForAnOrdinaryPath: a folder under a search root
// is used rather than one downloaded over it, whatever the path looks like.
func TestFolderBeatsFetchingForAnOrdinaryPath(t *testing.T) {
	root := t.TempDir()
	local := writePackage(t, filepath.Join(root, "util", "text"), "text", "public func n() -> Int32 { return 7 }\n")

	p := &fakePackages{dirs: map[string]string{"util/text": "/should/not/be/used"}}
	u, diags := compile(t, `
import "util/text"
func main() -> Int32 { return text.n() }
`, vsc.Options{PackagePaths: []string{root}, Packages: p})
	for _, d := range diags {
		t.Fatalf("compile: %v", d)
	}
	if len(p.asked) != 0 {
		t.Errorf("resolver was asked for %v, though the folder was there", p.asked)
	}
	if len(u.Packages) != 1 || u.Packages[0].Dir != local {
		t.Fatalf("compiled %+v, want %s", u.Packages, local)
	}
}

// TestFetchedWhenNoFolderHasIt is what makes `vsc run` work on a machine
// that has never seen the package.
func TestFetchedWhenNoFolderHasIt(t *testing.T) {
	fetched := writePackage(t, filepath.Join(t.TempDir(), "thing"), "thing", "public func n() -> Int32 { return 9 }\n")
	p := &fakePackages{dirs: map[string]string{"github.com/you/thing": fetched}}
	u, diags := compile(t, `
import "github.com/you/thing"
func main() -> Int32 { return thing.n() }
`, vsc.Options{Packages: p})
	for _, d := range diags {
		t.Fatalf("compile: %v", d)
	}
	if len(u.Packages) != 1 || u.Packages[0].Dir != fetched {
		t.Fatalf("compiled %+v, want %s", u.Packages, fetched)
	}
}

// TestRelativePathIsNeverFetched keeps "./util" meaning the folder beside
// the file, whatever a resolver would say about it.
func TestRelativePathIsNeverFetched(t *testing.T) {
	p := &fakePackages{dirs: map[string]string{"./util": "/somewhere"}}
	_, diags := compile(t, `
import "./util"
func main() -> Int32 { return 0 }
`, vsc.Options{Packages: p})
	if len(diags) == 0 {
		t.Fatal("a relative import of a folder that is not there was accepted")
	}
	if len(p.asked) != 0 || len(p.near) != 0 {
		t.Errorf("resolver was asked for %v and %v, though the path is relative", p.asked, p.near)
	}
}

// TestNoResolverStillReportsTheMissingPackage: compiling with nothing to
// fetch through is the ordinary case for a library, and the message is
// the one it always was.
func TestNoResolverStillReportsTheMissingPackage(t *testing.T) {
	_, diags := compile(t, `
import "net/tcp"
func main() -> Int32 { return 0 }
`, vsc.Options{})
	if len(diags) == 0 {
		t.Fatal("an import of a package that is not there was accepted")
	}
	if !strings.Contains(diags[0].Message, "no such package 'net/tcp'") {
		t.Errorf("reported %q", diags[0].Message)
	}
}

// TestAFetchThatFailsIsReported: with no folder and no checkout, what the
// fetch said is the diagnostic, not a generic "no such package".
func TestAFetchThatFailsIsReported(t *testing.T) {
	p := &fakePackages{errs: map[string]error{"util/text": os.ErrNotExist}}
	_, diags := compile(t, `
import "util/text"
func main() -> Int32 { return 0 }
`, vsc.Options{PackagePaths: []string{t.TempDir()}, Packages: p})
	if len(diags) == 0 {
		t.Fatal("an import nothing could find was accepted")
	}
	if !strings.Contains(diags[0].Message, os.ErrNotExist.Error()) {
		t.Errorf("reported %q, want the fetch's error", diags[0].Message)
	}
	if len(p.asked) != 1 || p.asked[0] != "util/text" {
		t.Errorf("resolver was asked %v, want [util/text]", p.asked)
	}
}
