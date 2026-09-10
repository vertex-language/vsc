package sysroot

import (
	"errors"
	"strings"
	"testing"
)

// The walk, driven as machines this one is not.
//
// Every case here runs on every host, which is the point of the Host
// interface: the answer for Windows is a walk over two versioned
// installations, and a walk that can only be exercised on a machine
// that has them is a walk nobody checks until it breaks.

// fake is a machine described rather than found. Dirs and files are
// keyed by the path as this package spells it — backslashes on
// Windows — and run answers one command.
type fake struct {
	env   map[string]string
	dirs  map[string]bool
	files map[string]string
	run   map[string]string
}

func (f fake) Getenv(key string) string { return f.env[key] }

// IsDir compares case-insensitively, which is what the filesystems
// being modelled do. %LIB% is written by an installer and shouted by
// some of them, so a fake that answered no to a path spelled in
// capitals would be stricter than any machine this runs on.
func (f fake) IsDir(path string) bool {
	if f.dirs[path] {
		return true
	}
	for d := range f.dirs {
		if strings.EqualFold(d, path) {
			return true
		}
	}
	return false
}

func (f fake) ReadDir(path string) ([]string, error) {
	prefix := path + `\`
	var out []string
	seen := map[string]bool{}
	for d := range f.dirs {
		if !strings.HasPrefix(d, prefix) {
			continue
		}
		name := strings.SplitN(strings.TrimPrefix(d, prefix), `\`, 2)[0]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no such directory")
	}
	return out, nil
}

func (f fake) ReadFile(path string) (string, error) {
	if v, ok := f.files[path]; ok {
		return v, nil
	}
	return "", errors.New("no such file")
}

func (f fake) Run(name string, args ...string) (string, error) {
	if v, ok := f.run[name]; ok {
		return v, nil
	}
	return "", errors.New("no such tool")
}

// dirsOf builds the dirs map, marking every ancestor a directory too,
// since a machine with C:\a\b\c has C:\a and C:\a\b.
func dirsOf(paths ...string) map[string]bool {
	out := map[string]bool{}
	for _, p := range paths {
		parts := strings.Split(p, `\`)
		for i := range parts {
			out[strings.Join(parts[:i+1], `\`)] = true
		}
	}
	return out
}

const (
	vsRoot  = `C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools`
	kitRoot = `C:\Program Files (x86)\Windows Kits\10`
)

// windowsMachine is a machine with both installations and no vcvars:
// an ordinary shell, which is what vsc is usually run from.
func windowsMachine() fake {
	return fake{
		env: map[string]string{"ProgramFiles(x86)": `C:\Program Files (x86)`},
		dirs: dirsOf(
			vsRoot+`\VC\Tools\MSVC\14.44.35207\lib\x64`,
			vsRoot+`\VC\Tools\MSVC\14.30.30705\lib\x64`,
			kitRoot+`\Lib\10.0.26100.0\ucrt\x64`,
			kitRoot+`\Lib\10.0.26100.0\um\x64`,
		),
		run: map[string]string{
			`C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe`: vsRoot + "\r\n",
		},
	}
}

// TestWindowsFromNothing is the case vsc exists for: no vcvars, no
// %LIB%, and both installations found anyway.
func TestWindowsFromNothing(t *testing.T) {
	got := libraryDirs(windowsMachine(), "x86_64-windows", true)
	want := []string{
		vsRoot + `\VC\Tools\MSVC\14.44.35207\lib\x64`,
		kitRoot + `\Lib\10.0.26100.0\ucrt\x64`,
		kitRoot + `\Lib\10.0.26100.0\um\x64`,
	}
	equal(t, got, want)
}

// TestWindowsNewestToolset: two toolsets side by side and the newest
// wins, compared numerically. Lexically, 14.30.30705 sorts above
// 14.44.35207 and is four years older.
func TestWindowsNewestToolset(t *testing.T) {
	got := libraryDirs(windowsMachine(), "x86_64-windows", true)
	for _, dir := range got {
		if strings.Contains(dir, "14.30.30705") {
			t.Errorf("picked the older toolset: %s", dir)
		}
	}
}

// TestWindowsDefaultVersionFile: the installation names its own
// toolset in Microsoft.VCToolsVersion.default.txt, which is what
// vcvars reads, and that answer beats newest-wins.
func TestWindowsDefaultVersionFile(t *testing.T) {
	h := windowsMachine()
	h.files = map[string]string{
		vsRoot + `\VC\Auxiliary\Build\Microsoft.VCToolsVersion.default.txt`: "14.30.30705\r\n",
	}
	got := libraryDirs(h, "x86_64-windows", true)
	if len(got) == 0 || !strings.Contains(got[0], "14.30.30705") {
		t.Errorf("did not honour the installation's own version file: %v", got)
	}
}

// TestWindowsEnvironmentFirst: inside a vcvars shell %LIB% is the
// answer and comes first, and what the walk finds is appended without
// repeating what is already there.
func TestWindowsEnvironmentFirst(t *testing.T) {
	h := windowsMachine()
	h.env["LIB"] = `C:\other\lib;` + strings.ToUpper(kitRoot+`\LIB\10.0.26100.0\UCRT\X64`) + `;`
	h.dirs[`C:\other\lib`] = true

	got := libraryDirs(h, "x86_64-windows", true)
	if len(got) == 0 || got[0] != `C:\other\lib` {
		t.Fatalf("the environment did not come first: %v", got)
	}
	// The ucrt directory was named by %LIB% in another case and
	// another spelling; it must appear once.
	n := 0
	for _, d := range got {
		if strings.EqualFold(strings.ReplaceAll(d, "/", `\`), kitRoot+`\Lib\10.0.26100.0\ucrt\x64`) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the ucrt directory appears %d times, want 1:\n%v", n, got)
	}
}

// TestWindowsUninstalledSDK: uninstalling an SDK leaves its version
// directory behind with the libraries gone. A newest-wins pick that
// did not look inside would choose the empty one and report every
// symbol undefined.
func TestWindowsUninstalledSDK(t *testing.T) {
	h := windowsMachine()
	for _, d := range []string{
		kitRoot + `\Lib\10.0.99999.0`,
		kitRoot + `\Lib\10.0.99999.0\um`,
	} {
		h.dirs[d] = true
	}
	got := libraryDirs(h, "x86_64-windows", true)
	for _, dir := range got {
		if strings.Contains(dir, "99999") {
			t.Errorf("chose an SDK with no ucrt: %s", dir)
		}
	}
}

// TestWindowsNothingInstalled: a machine with neither installation
// gets no directories and no invented ones, rather than a list of
// paths that are not there.
func TestWindowsNothingInstalled(t *testing.T) {
	h := fake{env: map[string]string{"ProgramFiles(x86)": `C:\Program Files (x86)`}}
	if got := libraryDirs(h, "x86_64-windows", true); len(got) != 0 {
		t.Errorf("invented %v", got)
	}
}

// TestFreestandingTakesNothing: a program that names no platform
// library is given no platform directories and no platform runtime,
// on either target.
func TestFreestandingTakesNothing(t *testing.T) {
	for _, target := range []string{"x86_64-windows", "aarch64-macos"} {
		if got := libraryDirs(windowsMachine(), target, false); len(got) != 0 {
			t.Errorf("%s: freestanding got dirs %v", target, got)
		}
		if got := DefaultLibraries(target, false); len(got) != 0 {
			t.Errorf("%s: freestanding got libs %v", target, got)
		}
	}
}

// TestCrossTargetIgnoresTheHost: which runtime a program needs is a
// fact about the target and not about the machine doing the linking.
// A Mach-O link never wants the MSVC runtime, wherever it is run.
func TestCrossTargetIgnoresTheHost(t *testing.T) {
	if got := DefaultLibraries("aarch64-macos", true); len(got) != 0 {
		t.Errorf("a Mach-O link asked for %v", got)
	}
	if got := DefaultLibraries("x86_64-windows", true); len(got) == 0 {
		t.Error("a Windows link asked for no C runtime")
	}
	// A target the table has never heard of gets nothing rather than
	// a guess.
	if got := DefaultLibraries("vax-ultrix", true); len(got) != 0 {
		t.Errorf("an unknown target asked for %v", got)
	}
}

// TestLibraryNames is MSVC's convention, which is the reverse of
// Unix's: foo.lib is the import library and libfoo.lib the static
// one. A name that already carries the prefix is what
// DefaultLibraries returns and must not grow a second one.
func TestLibraryNames(t *testing.T) {
	equal(t, LibraryNames("x86_64-windows", "kernel32"),
		[]string{"kernel32.lib", "libkernel32.lib"})
	equal(t, LibraryNames("x86_64-windows", "libucrt"), []string{"libucrt.lib"})
	equal(t, LibraryNames("aarch64-macos", "System"),
		[]string{"libSystem.tbd", "libSystem.dylib", "libSystem.a"})
}

// TestMSVCArch: both installations split their libraries by
// architecture and spell it their own way. A target neither has a
// name for gets no directories rather than a path with an empty
// component in it.
func TestMSVCArch(t *testing.T) {
	if got := msvcArch("x86_64-windows"); got != "x64" {
		t.Errorf("x86_64 = %q, want x64", got)
	}
	if got := msvcArch("riscv64-windows"); got != "" {
		t.Errorf("an unknown architecture answered %q", got)
	}
	h := windowsMachine()
	if got := libraryDirs(h, "riscv64-windows", true); len(got) != 0 {
		t.Errorf("invented %v", got)
	}
}

// TestDarwinSDKOrder is the lookup Apple's own tools use, in their
// order: SDKROOT, then xcrun, then the Command Line Tools path.
func TestDarwinSDKOrder(t *testing.T) {
	h := fake{env: map[string]string{"SDKROOT": "/from/env"}}
	if sdk, ok := darwinSDK(h); !ok || sdk != "/from/env" {
		t.Errorf("SDKROOT = %q %v", sdk, ok)
	}

	h = fake{run: map[string]string{"xcrun": "/from/xcrun\n"}}
	if sdk, ok := darwinSDK(h); !ok || sdk != "/from/xcrun" {
		t.Errorf("xcrun = %q %v", sdk, ok)
	}

	const clt = "/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk"
	h = fake{dirs: map[string]bool{clt: true}}
	if sdk, ok := darwinSDK(h); !ok || sdk != clt {
		t.Errorf("command line tools = %q %v", sdk, ok)
	}

	if _, ok := darwinSDK(fake{}); ok {
		t.Error("found an SDK on a machine with none")
	}
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got  %v\nwant %v", got, want)
		}
	}
}
