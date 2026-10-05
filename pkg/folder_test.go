package pkg

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestForTarget(t *testing.T) {
	for _, c := range []struct {
		name string
		want bool
	}{
		{"sock.cpp", true},
		{"sock_posix.cpp", true},
		{"sock_windows.cpp", false},
		{"sock_darwin.cpp", true},
		{"sock_linux.cpp", false},
		{"simd_arm64.cpp", true},
		{"simd_amd64.cpp", false},
		{"simd_darwin_arm64.cpp", true},
		{"simd_linux_arm64.cpp", false},
		{"my_helper.vs", true},
	} {
		if got := ForTarget(c.name, "macos", "arm64"); got != c.want {
			t.Errorf("ForTarget(%s, macos, arm64) = %v, want %v", c.name, got, c.want)
		}
	}
	if ForTarget("sock_posix.cpp", "windows", "amd64") {
		t.Error("a _posix file is built for windows")
	}
}

func TestReadFolder(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.vs", "b_windows.vs", "sock.cpp", "sock_posix.cpp", "sock_windows.cpp", "win.mm", "notes.md"} {
		mustWrite(t, filepath.Join(dir, f), "")
	}
	f, err := ReadFolder(dir, "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	base := func(paths []string) string {
		var out []string
		for _, p := range paths {
			out = append(out, filepath.Base(p))
		}
		return strings.Join(out, " ")
	}
	if got := base(f.Vertex); got != "a.vs" {
		t.Errorf("Vertex = %s", got)
	}
	if got := base(f.Native); got != "sock.cpp sock_posix.cpp win.mm" {
		t.Errorf("Native = %s", got)
	}

	for _, bad := range []string{"old.c", "old.m"} {
		d := t.TempDir()
		mustWrite(t, filepath.Join(d, bad), "")
		if _, err := ReadFolder(d, "macos", "arm64"); err == nil {
			t.Errorf("a folder with %s read", bad)
		}
	}
}

func TestParseModFile(t *testing.T) {
	dir := t.TempDir()
	src := `// the network stack
module github.com/vertex-language/net

vertex 0.9
platform macos 13

require (
	github.com/vertex-language/io v0.4.0
	github.com/you/thing v1.2.3 // pinned
)

replace github.com/vertex-language/io => ../io
replace github.com/you/thing v1.2.3 => github.com/me/thing v1.2.4
`
	m, err := ParseModFile(filepath.Join(dir, "vs.mod"), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.Module != "net" || m.Vertex != "0.9" || m.Platforms["macos"] != "13" {
		t.Errorf("header = %+v", m)
	}
	if m.Required("io") != "v0.4.0" || m.Required("github.com/you/thing") != "v1.2.3" {
		t.Errorf("require = %+v", m.Require)
	}
	if len(m.Replace) != 2 || m.Replace[0].Module != "io" || m.Replace[0].Dir != filepath.Join(filepath.Dir(dir), "io") {
		t.Errorf("replace[0] = %+v", m.Replace)
	}
	if r := m.Replace[1]; r.Version != "v1.2.3" || r.New.Module != "github.com/me/thing" || r.New.Version != "v1.2.4" {
		t.Errorf("replace[1] = %+v", r)
	}

	for _, bad := range []string{"vertex 0.9\n", "module a\nfrobnicate x\n", "module a\nrequire (\n x v1\n"} {
		if _, err := ParseModFile("vs.mod", []byte(bad)); err == nil {
			t.Errorf("parsed %q", bad)
		}
	}
}

func TestWorkFile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "vs.work"), "vertex 0.9\nuse (\n\t./io\n\t./net\n)\n")
	mustWrite(t, filepath.Join(root, "net", "tcp", "x.vs"), "")
	path := FindWorkFile(filepath.Join(root, "net", "tcp"))
	if path != filepath.Join(root, "vs.work") {
		t.Fatalf("FindWorkFile = %q", path)
	}
	w, err := LoadWorkFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Use) != 2 || w.Use[1] != filepath.Join(root, "net") {
		t.Errorf("use = %v", w.Use)
	}
}
