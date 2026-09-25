package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModFileName is the file a Vertex module is rooted at. It is optional: a
// repository without one is still a module, named by where it is fetched
// from, and its imports are taken at their default branches.
const ModFileName = "vs.mod"

// SumFileName holds the hashes of the modules a vs.mod requires.
const SumFileName = "vs.sum"

// WorkFileName names local modules to use in place of fetched ones, for
// working on several repositories at once.
const WorkFileName = "vs.work"

// stdlibHost is the prefix a standard library module's full path has and
// its import paths leave out: github.com/vertex-language/net is imported as
// "net/tcp".
const stdlibHost = "github.com/vertex-language/"

// A ModFile is a vs.mod. Its syntax is go.mod's.
type ModFile struct {
	Path string
	// Module is the module's import path: "net" for
	// github.com/vertex-language/net, the full path for any other.
	Module string
	// Vertex is the toolchain version the module is written against.
	Vertex string
	// Platforms are the deployment targets: "macos" → "13".
	Platforms map[string]string
	Require   []Require
	Replace   []Replace
	Exclude   []Require
}

// A Require is a module at a version.
type Require struct {
	Module  string
	Version string
}

// A Replace stands a local directory, or another module version, in for a
// module. Dir is absolute where the replacement is a directory.
type Replace struct {
	Module  string
	Version string // "" replaces every version
	Dir     string
	New     Require
}

// A WorkFile is a vs.work: modules on disk that stand in for their
// fetched versions, for every build under the directory it is in.
type WorkFile struct {
	Path    string
	Vertex  string
	Use     []string // absolute directories
	Replace []Replace
}

// ImportPath is how a module path is written in an import: the standard
// library's without its host.
func ImportPath(module string) string {
	return strings.TrimPrefix(module, stdlibHost)
}

// FindModFile is the vs.mod dir is rooted at, or "".
func FindModFile(dir string) string {
	p := filepath.Join(dir, ModFileName)
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return p
	}
	return ""
}

// LoadModFile reads the vs.mod at path.
func LoadModFile(path string) (*ModFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseModFile(path, src)
}

// ParseModFile parses a vs.mod.
func ParseModFile(path string, src []byte) (*ModFile, error) {
	m := &ModFile{Path: path, Platforms: map[string]string{}}
	dir := filepath.Dir(path)
	err := eachDirective(path, src, func(line int, verb string, args []string) error {
		switch verb {
		case "module":
			if len(args) != 1 {
				return fmt.Errorf("module takes one path")
			}
			m.Module = ImportPath(args[0])
		case "vertex":
			if len(args) != 1 {
				return fmt.Errorf("vertex takes one version")
			}
			m.Vertex = args[0]
		case "platform":
			if len(args) != 2 {
				return fmt.Errorf("platform takes a name and a version: platform macos 13")
			}
			m.Platforms[args[0]] = args[1]
		case "require", "exclude":
			if len(args) != 2 {
				return fmt.Errorf("%s takes a module and a version", verb)
			}
			r := Require{Module: ImportPath(args[0]), Version: args[1]}
			if verb == "require" {
				m.Require = append(m.Require, r)
			} else {
				m.Exclude = append(m.Exclude, r)
			}
		case "replace":
			r, err := parseReplace(dir, args)
			if err != nil {
				return err
			}
			m.Replace = append(m.Replace, r)
		default:
			return fmt.Errorf("unknown directive %q", verb)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if m.Module == "" {
		return nil, fmt.Errorf("%s: no module line: it must say which module this is", path)
	}
	return m, nil
}

// Required is the version the module requires of mod, or "".
func (m *ModFile) Required(mod string) string {
	for _, r := range m.Require {
		if r.Module == mod {
			return r.Version
		}
	}
	return ""
}

// FindWorkFile is the vs.work in dir or the nearest directory above it,
// or "".
func FindWorkFile(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for d := abs; ; d = filepath.Dir(d) {
		p := filepath.Join(d, WorkFileName)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// LoadWorkFile reads the vs.work at path.
func LoadWorkFile(path string) (*WorkFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	w := &WorkFile{Path: path}
	dir := filepath.Dir(path)
	err = eachDirective(path, src, func(line int, verb string, args []string) error {
		switch verb {
		case "vertex":
			if len(args) != 1 {
				return fmt.Errorf("vertex takes one version")
			}
			w.Vertex = args[0]
		case "use":
			if len(args) != 1 {
				return fmt.Errorf("use takes one directory")
			}
			w.Use = append(w.Use, absUnder(dir, args[0]))
		case "replace":
			r, err := parseReplace(dir, args)
			if err != nil {
				return err
			}
			w.Replace = append(w.Replace, r)
		default:
			return fmt.Errorf("unknown directive %q", verb)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return w, nil
}

// parseReplace reads `old [v] => new [v]`, where new is a directory when it
// starts with ./, ../ or / and a module otherwise.
func parseReplace(dir string, args []string) (Replace, error) {
	arrow := -1
	for i, a := range args {
		if a == "=>" {
			arrow = i
		}
	}
	if arrow < 1 || arrow > 2 || len(args)-arrow-1 < 1 || len(args)-arrow-1 > 2 {
		return Replace{}, fmt.Errorf("replace is `module [version] => directory` or `module [version] => module version`")
	}
	r := Replace{Module: ImportPath(args[0])}
	if arrow == 2 {
		r.Version = args[1]
	}
	to := args[arrow+1:]
	if isLocalPath(to[0]) {
		if len(to) != 1 {
			return Replace{}, fmt.Errorf("a directory replacement takes no version")
		}
		r.Dir = absUnder(dir, to[0])
		return r, nil
	}
	if len(to) != 2 {
		return Replace{}, fmt.Errorf("a module replacement needs a version")
	}
	r.New = Require{Module: ImportPath(to[0]), Version: to[1]}
	return r, nil
}

func isLocalPath(s string) bool {
	return strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || filepath.IsAbs(s) || s == "." || s == ".."
}

func absUnder(dir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(dir, filepath.FromSlash(p))
}

// eachDirective walks go.mod syntax: `verb args...` lines, `verb ( ... )`
// blocks of argument lines, and // comments.
func eachDirective(path string, src []byte, do func(line int, verb string, args []string) error) error {
	block := ""
	for i, raw := range strings.Split(string(src), "\n") {
		line := raw
		if j := strings.Index(line, "//"); j >= 0 {
			line = line[:j]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		wrap := func(err error) error {
			return fmt.Errorf("%s:%d: %v", path, i+1, err)
		}
		if block != "" {
			if fields[0] == ")" && len(fields) == 1 {
				block = ""
				continue
			}
			if err := do(i+1, block, fields); err != nil {
				return wrap(err)
			}
			continue
		}
		if len(fields) == 2 && fields[1] == "(" {
			block = fields[0]
			continue
		}
		if err := do(i+1, fields[0], fields[1:]); err != nil {
			return wrap(err)
		}
	}
	if block != "" {
		return fmt.Errorf("%s: the %s block is not closed", path, block)
	}
	return nil
}
