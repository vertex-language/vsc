// Package sysroot resolves system library search paths and platform runtime
// libraries for supported target platforms.
package sysroot

import (
	"os"
	"os/exec"
	"strings"
)

// Host abstracts filesystem and environment operations to allow test mocking.
type Host interface {
	// Getenv returns the named variable, or "" when unset.
	Getenv(key string) string
	// IsDir reports whether path exists and is a directory.
	IsDir(path string) bool
	// ReadDir returns the names of the entries in a directory.
	ReadDir(path string) ([]string, error)
	// ReadFile returns the contents of a file.
	ReadFile(path string) (string, error)
	// Run executes a command and returns its standard output.
	Run(name string, args ...string) (string, error)
}

// osHost is the real host machine. A nil Host defaults to osHost.
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

// LibraryDirs returns the ordered search paths for libraries on the target platform.
// A nil Host defaults to osHost.
func LibraryDirs(h Host, target string, hosted bool) []string {
	return libraryDirs(host(h), target, hosted)
}

func libraryDirs(h Host, target string, hosted bool) []string {
	if !hosted {
		return nil
	}
	var dirs []string
	switch {
	case IsWindows(target):
		dirs = windowsLibraryDirs(h, target)
	case IsMacOS(target):
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

// FrameworkDirs returns where frameworks are searched for on target: the
// SDK's public frameworks on macOS, and nothing anywhere else.
func FrameworkDirs(h Host, target string, hosted bool) []string {
	h = host(h)
	if !hosted || !IsMacOS(target) {
		return nil
	}
	sdk, ok := darwinSDK(h)
	if !ok {
		return nil
	}
	dir := sdk + "/System/Library/Frameworks"
	if !h.IsDir(dir) {
		return nil
	}
	return []string{dir}
}

// DefaultLibraries returns the default platform runtime libraries for target.
func DefaultLibraries(target string, hosted bool) []string {
	if !hosted || !IsWindows(target) {
		return nil
	}
	return []string{"libucrt", "libvcruntime", "libcmt", "kernel32"}
}

// LibraryNames returns the file name variants tried for a given library name on target.
func LibraryNames(target, name string) []string {
	switch {
	case IsWindows(target):
		if strings.HasPrefix(name, "lib") {
			return []string{name + ".lib"}
		}
		return []string{name + ".lib", "lib" + name + ".lib"}
	case IsMacOS(target):
		return []string{"lib" + name + ".tbd", "lib" + name + ".dylib", "lib" + name + ".a"}
	case strings.HasSuffix(target, "-android"):
		return []string{"lib" + name + ".so", "lib" + name + ".a"}
	}
	return []string{"lib" + name + ".a"}
}

// IsWindows reports whether target is a Windows platform.
func IsWindows(target string) bool { return strings.HasSuffix(target, "-windows") }

// IsMacOS reports whether target is a macOS platform.
func IsMacOS(target string) bool { return strings.HasSuffix(target, "-macos") }

// archOf is the architecture half of a target name.
func archOf(target string) string {
	if i := strings.IndexByte(target, '-'); i >= 0 {
		return target[:i]
	}
	return target
}
