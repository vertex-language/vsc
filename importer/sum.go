package importer

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Hash is a checkout's tree hash as vs.sum records it: Go's dirhash "h1",
// a SHA-256 over the sorted list of each file's SHA-256 and path. The
// .git directory is not part of the tree.
func Hash(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".build" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	summary := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(summary, "%x  %s\n", sha256.Sum256(data), f)
	}
	return "h1:" + base64.StdEncoding.EncodeToString(summary.Sum(nil)), nil
}

// A Sums is a vs.sum: each module version's tree hash.
type Sums struct {
	Path    string
	entries map[string]string // "module version" → hash
	changed bool
}

// ReadSums reads the vs.sum at path; a missing file is an empty one.
func ReadSums(path string) (*Sums, error) {
	s := &Sums{Path: path, entries: map[string]string{}}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 3 {
			return nil, fmt.Errorf("%s:%d: a line is `module version hash`", path, n)
		}
		s.entries[fields[0]+" "+fields[1]] = fields[2]
	}
	return s, sc.Err()
}

// Check compares a module version's hash with the one recorded, and
// records it where there is none. A mismatch is an error: what was fetched
// is not what the module was built against.
func (s *Sums) Check(module, version, hash string) error {
	key := module + " " + version
	if have, ok := s.entries[key]; ok {
		if have != hash {
			return fmt.Errorf("%s %s: its hash is %s, but %s records %s: the repository changed under this version",
				module, version, hash, filepath.Base(s.Path), have)
		}
		return nil
	}
	s.entries[key] = hash
	s.changed = true
	return nil
}

// Save writes the file back if Check recorded anything new.
func (s *Sums) Save() error {
	if !s.changed {
		return nil
	}
	keys := make([]string, 0, len(s.entries))
	for k := range s.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %s\n", k, s.entries[k])
	}
	s.changed = false
	return os.WriteFile(s.Path, []byte(b.String()), 0o644)
}

// ModuleOf is the module -- the repository -- an import path is in, as a
// vs.mod names it: "net" for net/tcp, github.com/you/thing for
// github.com/you/thing/x.
func ModuleOf(path string) string {
	m, ok := Lookup(path)
	if !ok {
		return path
	}
	if m.Stdlib {
		slug, _, _ := StdlibSlug(m.Path)
		return slug
	}
	parts := strings.Split(m.Path, "/")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, "/")
}
