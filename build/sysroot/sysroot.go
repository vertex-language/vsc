// Package sysroot answers the one question the linker cannot: where
// does this host keep the target's libraries?
//
// LibraryDirs is the ordered search path a library name is resolved
// against, and DefaultLibraries is what a hosted link needs when the
// program named nothing. Both are data — build/link.go turns them
// into bytes, and `vsc env` prints them before a build runs, so a
// link that fails for want of a toolchain says so before it fails.
//
// It is vcc's sysroot package reduced to the half a compiler with no
// preprocessor needs. vcc resolves an include list here too, since a
// C compilation reads the platform's headers; a Vertex one reads
// none, so that half is absent rather than unwritten.
//
// This package imports the standard library only.
package sysroot

import (
	"os"
	"os/exec"
	"strings"
)

// Host is everything this package reads from the machine: environment
// variables, directory existence and contents, one small file, and one
// tool's output (xcrun, vswhere).
//
// The indirection exists because probing is impure by nature and the
// ordering logic is not. With Host injected, the walk is a function a
// test can drive as a Mac with no Xcode or a Windows shell with no
// vcvars, on any machine — which matters most for the platform whose
// answer is hardest to reach.
//
// ReadDir and ReadFile exist for one platform. macOS names its SDK
// directory outright; Windows versions both of its installations, so
// the toolset and the SDK live under a directory whose name is a
// version number that has to be read to be known.
type Host interface {
	// Getenv returns the named variable, "" when unset.
	Getenv(key string) string
	// IsDir reports whether path exists and is a directory.
	IsDir(path string) bool
	// ReadDir returns the names of the entries in a directory, in any
	// order. An unreadable directory returns no names and an error.
	ReadDir(path string) ([]string, error)
	// ReadFile returns the contents of a small file.
	ReadFile(path string) (string, error)
	// Run executes a tool and returns its standard output.
	Run(name string, args ...string) (string, error)
}

// osHost is the real machine. A nil Host means this one, everywhere
// in this package, so a caller who is not a test never names it.
type osHost struct{}

func (osHost) Getenv(key string) string { return os.Getenv(key) }

func (osHost) IsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func (osHost) ReadDir(path string) ([]string, error) {
	ents, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names, nil
}

func (osHost) ReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func (osHost) Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

func host(h Host) Host {
	if h == nil {
		return osHost{}
	}
	return h
}

// LibraryDirs is where this host keeps the target's libraries, in the
// order a library name is looked for.
//
// Directories are probed rather than assumed, so an absent one is an
// absent entry and no installation table decides anything. A
// freestanding link gets nothing: a program that names no platform
// library names its own.
//
// The target decides which walk runs, not the machine doing the
// linking, so a cross-link asks the same questions a native one does
// and gets no answer where the installation is not there to find.
//
// A nil Host is the real machine.
func LibraryDirs(h Host, target string, hosted bool) []string {
	return libraryDirs(host(h), target, hosted)
}

// libraryDirs is LibraryDirs with the machine injected. Tests call
// this; nothing else should.
func libraryDirs(h Host, target string, hosted bool) []string {
	if !hosted {
		return nil
	}
	var dirs []string
	switch {
	case IsWindows(target):
		// Windows is the one target whose libraries are found rather
		// than named, and windows.go is that walk. It takes the
		// target because both installations split their libraries by
		// architecture.
		dirs = windowsLibraryDirs(h, target)
	case IsMacOS(target):
		// The SDK is the only place a Mach-O link finds a system
		// library: /usr/lib holds no dylibs on a modern macOS, since
		// the shared cache replaced them, and what a link reads is
		// the .tbd stub inside the SDK.
		dirs = append(dirs, "/usr/local/lib")
		if sdk, ok := darwinSDK(h); ok {
			dirs = append(dirs, sdk+"/usr/lib")
		}
	}

	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if h.IsDir(dir) {
			out = append(out, dir)
		}
	}
	return out
}

// DefaultLibraries is what a hosted link is given when the program
// named nothing: the libraries it still has to be linked against for
// its entry point to be reached and for malloc to exist.
//
// It is the platform's answer rather than a preference, which is why
// it lives beside LibraryDirs rather than in the linker.
//
// Only Windows has an answer, and the reason macOS has none is not
// that it needs nothing: it needs libSystem, whose stub the Mach-O
// link adds by path because it is one file at a known place inside
// the SDK rather than a name to resolve. See build/link.go.
func DefaultLibraries(target string, hosted bool) []string {
	if !hosted || !IsWindows(target) {
		return nil
	}
	// The static CRT, which is cl.exe's own default and the one that
	// needs nothing installed to run: ucrtbase.dll ships with Windows
	// but vcruntime140.dll does not, so a link against the DLL
	// runtime produces a program that runs on the machine that built
	// it and nowhere else.
	//
	// The names carry the lib prefix because MSVC's convention is the
	// reverse of Unix's — foo.lib is the import library that binds to
	// foo.dll and libfoo.lib is the static one — so spelling the
	// static form outright is the only way to say which is meant
	// without leaning on the search order.
	//
	// libcmt is what supplies mainCRTStartup, which is where the
	// image begins and which calls the main this compiler emitted;
	// libucrt is malloc and free, which the Vertex runtime calls;
	// libvcruntime is what the CRT itself needs; and kernel32 is what
	// all three call in turn.
	return []string{"libucrt", "libvcruntime", "libcmt", "kernel32"}
}

// LibraryNames is the filenames a library name can mean on this
// target, in the order they are tried.
//
// MSVC's convention is the reverse of Unix's, and this is the whole
// of it: foo.lib is the import library that binds to foo.dll and
// libfoo.lib is the static one. There is no ".a" in a Microsoft
// toolchain at all, which is why a name that already carries the lib
// prefix — every one DefaultLibraries returns — resolves as written
// rather than growing a second one.
//
// Mach-O looks for a .tbd first because that is what a modern macOS
// SDK ships and what the linker can actually read: the dylib itself
// lives in the shared cache.
func LibraryNames(target, name string) []string {
	switch {
	case IsWindows(target):
		if strings.HasPrefix(name, "lib") {
			return []string{name + ".lib"}
		}
		return []string{name + ".lib", "lib" + name + ".lib"}
	case IsMacOS(target):
		return []string{"lib" + name + ".tbd", "lib" + name + ".dylib", "lib" + name + ".a"}
	}
	return []string{"lib" + name + ".a"}
}

// The target names this package recognises. They are vsc's, which are
// the family's: arch-os, and the half that decides everything here is
// the second one.
func IsWindows(target string) bool { return strings.HasSuffix(target, "-windows") }
func IsMacOS(target string) bool   { return strings.HasSuffix(target, "-macos") }

// archOf is the architecture half of a target name.
func archOf(target string) string {
	if i := strings.IndexByte(target, '-'); i >= 0 {
		return target[:i]
	}
	return target
}
