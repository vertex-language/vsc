package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A Folder is a Vertex package as it is on disk: one directory, whose
// Vertex files are the package and whose C++ files are its native half.
// Nothing names the files; the folder is the package, as a Go package is.
type Folder struct {
	Dir string
	// Vertex are the .vs files built for the platform, in name order.
	Vertex []string
	// Native are the .cpp and .mm files built for the platform, in name
	// order: the package's C++ module, which vcx compiles.
	Native []string
}

// ReadFolder lists the sources of the package in dir that a target builds:
// platform is a manifest platform name ("macos"), arch a Go-style one
// ("arm64"). A file named for another platform or architecture is left
// out (see ForTarget). A .c or .m file is an error: a Vertex package's
// native language is C++.
func ReadFolder(dir, platform, arch string) (*Folder, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	f := &Folder{Dir: dir}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		switch strings.ToLower(filepath.Ext(name)) {
		case ".vs":
			if ForTarget(path, platform, arch) {
				f.Vertex = append(f.Vertex, path)
			}
		case ".cpp", ".cc", ".cxx", ".cppm", ".mm":
			if ForTarget(path, platform, arch) {
				f.Native = append(f.Native, path)
			}
		case ".c":
			return nil, fmt.Errorf("%s is C: a Vertex package's native code is C++, so port it to a .cpp", path)
		case ".m":
			return nil, fmt.Errorf("%s is Objective-C: a Vertex package's native code is C++, so port it to Objective-C++ as a .mm", path)
		}
	}
	sort.Strings(f.Vertex)
	sort.Strings(f.Native)
	return f, nil
}

// Empty reports whether the folder has no sources for the target.
func (f *Folder) Empty() bool { return len(f.Vertex) == 0 && len(f.Native) == 0 }

// sourceArchs are the architecture suffixes a file name may end in, by the
// names Go gives them, and the target architecture each is.
var sourceArchs = map[string]string{
	"arm64": "arm64",
	"amd64": "amd64",
}

// ForTarget reports whether the source at path is built for platform and
// arch. Its name may end in a platform (_darwin, _posix), an architecture
// (_arm64), or a platform then an architecture (_linux_amd64), as Go's
// file names do; a name with neither is built everywhere.
func ForTarget(path, platform, arch string) bool {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return true
	}
	last := parts[len(parts)-1]
	if a, ok := sourceArchs[last]; ok {
		if a != arch {
			return false
		}
		parts = parts[:len(parts)-1]
		if len(parts) < 2 {
			return true
		}
		last = parts[len(parts)-1]
	}
	plats, ok := sourcePlatforms[last]
	if !ok {
		return true
	}
	for _, p := range plats {
		if p == platform {
			return true
		}
	}
	return false
}

// ArchOf is the Go-style architecture name of an IR target's use path:
// "aarch64/macos" is "arm64".
func ArchOf(use string) string {
	switch {
	case strings.HasPrefix(use, "aarch64/"):
		return "arm64"
	case strings.HasPrefix(use, "x86_64/"):
		return "amd64"
	}
	return ""
}
