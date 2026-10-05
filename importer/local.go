package importer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/vsc/pkg"
)

// A source file belongs to a module: the checkout it is in. Imports of that
// module -- the module itself, or any folder of it -- are answered by the
// checkout, never by a download, so a package's programs and tests build
// against the package as it is on disk, and one folder of a repository
// importing another gets the same revision of it. It is Go's rule for the
// main module.

// A Root is the checkout a directory is inside, and the import path it
// answers to.
type Root struct {
	// Dir is the checkout: the nearest directory holding a vs.mod or a
	// .git.
	Dir string
	// Path is the import path of the checkout as a whole: "net" for
	// github.com/vertex-language/net, "github.com/you/thing" for any other
	// repository.
	Path string
	// Mod is the checkout's vs.mod, nil where it has none.
	Mod *pkg.ModFile
}

// Enclosing is the checkout dir is inside, if it is inside one.
//
// The checkout's import path is what it says of itself: its vs.mod's
// module line; failing that its origin remote, which is the URL it would
// be fetched from; failing that the directory's name.
func Enclosing(dir string) (Root, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Root{}, false
	}
	for d := abs; ; d = filepath.Dir(d) {
		modPath := pkg.FindModFile(d)
		git := isDir(filepath.Join(d, ".git"))
		if modPath != "" || git {
			r := Root{Dir: d}
			if modPath != "" {
				if m, err := pkg.LoadModFile(modPath); err == nil {
					r.Mod, r.Path = m, m.Module
				}
			}
			if r.Path == "" && git {
				r.Path = pathOfRemote(originURL(filepath.Join(d, ".git", "config")))
			}
			if r.Path == "" {
				r.Path = filepath.Base(d)
			}
			return r, true
		}
		if filepath.Dir(d) == d {
			return Root{}, false
		}
	}
}

// Local is the directory of source for an import path written in a file in
// fromDir, where the path is the module that file is in or a folder of it,
// and "" where it is not.
func Local(path, fromDir string) (string, error) {
	path = strings.Trim(path, "/")
	if path == "" || strings.HasPrefix(path, ".") || strings.Contains(path, "@") {
		return "", nil
	}
	r, ok := Enclosing(fromDir)
	if !ok || (path != r.Path && !strings.HasPrefix(path, r.Path+"/")) {
		return "", nil
	}
	m := Module{Path: path}
	if looked, ok := Lookup(path); ok {
		m = looked
	}
	// The folder the path names inside the checkout, whatever kind of
	// path it is: what is left after the checkout's own path.
	return sourceDirIn(m, r.Dir, strings.TrimPrefix(strings.TrimPrefix(path, r.Path), "/"))
}

// originURL is the URL of the origin remote a git config names, or "".
func originURL(config string) string {
	f, err := os.Open(config)
	if err != nil {
		return ""
	}
	defer f.Close()
	inOrigin := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if !inOrigin {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) == "url" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// pathOfRemote is the import path a repository URL is fetched under:
// https://github.com/vertex-language/net is "net", and
// git@github.com:you/thing.git is "github.com/you/thing". "" for a URL that
// names no repository this way.
func pathOfRemote(url string) string {
	if url == "" {
		return ""
	}
	u := strings.TrimSuffix(strings.TrimSuffix(url, "/"), ".git")
	switch {
	case strings.HasPrefix(u, "https://"):
		u = strings.TrimPrefix(u, "https://")
	case strings.HasPrefix(u, "http://"):
		u = strings.TrimPrefix(u, "http://")
	case strings.HasPrefix(u, "ssh://"):
		u = strings.TrimPrefix(u, "ssh://")
		if _, rest, ok := strings.Cut(u, "@"); ok {
			u = rest
		}
	case strings.Contains(u, "@") && strings.Contains(u, ":"):
		// scp-like: git@github.com:you/thing
		_, rest, _ := strings.Cut(u, "@")
		u = strings.Replace(rest, ":", "/", 1)
	default:
		return ""
	}
	parts := strings.Split(u, "/")
	if len(parts) != 3 || !strings.Contains(parts[0], ".") {
		return ""
	}
	if "https://"+strings.Join(parts[:2], "/") == StdlibOrg {
		return parts[2]
	}
	return u
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
