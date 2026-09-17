package importer

import "strings"

// A Module is where an import path's source comes from.
type Module struct {
	// Path is the import path as it is written: "net/tcp".
	Path string
	// Repo is the repository holding it, as a URL to clone.
	Repo string
	// Ref is the branch or tag to take, empty for the default branch.
	Ref string
	// Stdlib is whether the path is a standard library package -- one that
	// names no host, and lives in the vertex-language organization -- rather
	// than one that names its repository itself.
	Stdlib bool
}

// StdlibOrg is where the standard library's repositories are: one
// repository per first segment of a path, as github.com/vertex-language/net
// holds net/tcp, net/udp and net/http.
const StdlibOrg = "https://github.com/vertex-language"

// StdlibSlug splits a standard library path into its repository and the
// folder inside it: "net/tcp" is net and tcp, "time" is time and "". It is
// false for a path that is not one: relative, or naming a host.
func StdlibSlug(path string) (slug string, subpath string, ok bool) {
	path = strings.Trim(path, "/")
	if path == "" || strings.HasPrefix(path, ".") {
		return "", "", false
	}
	parts := strings.Split(path, "/")
	if strings.Contains(parts[0], ".") {
		return "", "", false
	}
	slug = parts[0]
	if len(parts) > 1 {
		subpath = strings.Join(parts[1:], "/")
	}
	return slug, subpath, true
}

// IsStdlib reports whether path is a standard library path.
func IsStdlib(path string) bool {
	_, _, ok := StdlibSlug(path)
	return ok
}

// StdlibExamples are standard library paths, for a message that has to
// show some.
func StdlibExamples() []string {
	return []string{"net/tcp", "time", "crypto/sha256"}
}

// Lookup is where an import path's source comes from, and whether it
// names one at all.
//
// A path whose first segment has a dot is a repository already --
// "github.com/you/thing", with anything after the third segment a folder
// inside it -- and is taken at its word, over https. Any other path is the
// standard library's: its first segment is the repository in StdlibOrg,
// and the rest a folder inside it.
func Lookup(path string) (Module, bool) {
	path = strings.Trim(path, "/")
	if path == "" || strings.HasPrefix(path, ".") {
		return Module{}, false
	}
	// A version or branch may be pinned with an @, as the cache records it.
	ref := ""
	if at := strings.LastIndex(path, "@"); at > 0 {
		ref = path[at+1:]
		path = path[:at]
	}
	if host, _, ok := strings.Cut(path, "/"); ok && strings.Contains(host, ".") {
		// The first three segments are the repository; anything after
		// them is a directory inside it.
		parts := strings.Split(path, "/")
		if len(parts) < 3 {
			return Module{}, false
		}
		repo := strings.Join(parts[:3], "/")
		return Module{Path: path, Repo: "https://" + repo, Ref: ref}, true
	}
	if slug, _, ok := StdlibSlug(path); ok {
		return Module{
			Path:   path,
			Repo:   StdlibOrg + "/" + slug,
			Ref:    ref,
			Stdlib: true,
		}, true
	}
	return Module{}, false
}

// subdir is the part of an import path below the repository, or "" when
// the path is the whole of it.
func (m Module) subdir() string {
	if m.Stdlib {
		_, sub, _ := StdlibSlug(m.Path)
		return sub
	}
	parts := strings.Split(m.Path, "/")
	if len(parts) <= 3 {
		return ""
	}
	return strings.Join(parts[3:], "/")
}
