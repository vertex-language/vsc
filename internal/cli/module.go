package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc/importer"
	"github.com/vertex-language/vsc/pkg"
)

// A mainModule is the module a build is of: the checkout its program's
// files are in, what that checkout's vs.mod requires and replaces, and
// the vs.work above it, if any. Go calls this the main module, and only
// its vs.mod decides versions.
type mainModule struct {
	root importer.Root
	// uses are the modules a vs.work puts on disk: import path → checkout.
	uses map[string]string
	// replaces are the vs.work's and vs.mod's directory replacements.
	replaces []pkg.Replace
	sums     *importer.Sums
}

// mainModule is the module the files in dir are part of, found once per
// build: the first directory asked about is the program's.
func (c *common) mainModule(dir string) *mainModule {
	if c.main != nil {
		return c.main
	}
	root, ok := importer.Enclosing(dir)
	if !ok {
		root = importer.Root{Dir: dir}
	}
	m := &mainModule{root: root, uses: map[string]string{}}
	if w := pkg.FindWorkFile(dir); w != "" {
		if work, err := pkg.LoadWorkFile(w); err == nil {
			for _, use := range work.Use {
				if r, ok := importer.Enclosing(use); ok && r.Dir == filepath.Clean(use) {
					m.uses[r.Path] = r.Dir
				} else {
					m.uses[filepath.Base(use)] = use
				}
			}
			m.replaces = append(m.replaces, work.Replace...)
		} else if c.notice != nil {
			c.notice(err.Error())
		}
	}
	if root.Mod != nil {
		m.replaces = append(m.replaces, root.Mod.Replace...)
	}
	c.main = m
	return m
}

// local is the checkout on disk the main module says to use for path: one
// a vs.work uses, or a directory a replace names.
func (m *mainModule) local(path string) (dir, sub string, ok bool) {
	for module, dir := range m.uses {
		if sub, ok := under(path, module); ok {
			return dir, sub, true
		}
	}
	for _, r := range m.replaces {
		if r.Dir == "" {
			continue
		}
		if sub, ok := under(path, r.Module); ok {
			return r.Dir, sub, true
		}
	}
	return "", "", false
}

// check holds a fetched module version to the hash the main module's
// vs.sum records for it, recording it the first time.
func (m *mainModule) check(module, version, dir, path string) error {
	if m.sums == nil {
		sums, err := importer.ReadSums(filepath.Join(m.root.Dir, pkg.SumFileName))
		if err != nil {
			return err
		}
		m.sums = sums
	}
	// The hash is of the repository, and dir is the folder of it the
	// path names.
	root := dir
	if sub, _ := under(path, module); sub != "" {
		root = filepath.Clean(strings.TrimSuffix(dir, filepath.FromSlash(sub)))
	}
	hash, err := importer.Hash(root)
	if err != nil {
		return err
	}
	if err := m.sums.Check(module, version, hash); err != nil {
		return err
	}
	return m.sums.Save()
}

// minOS is the macOS version the main module's vs.mod says it runs on, or
// "" where it says nothing.
func (m *mainModule) minOS(target ir.Target) string {
	if m == nil || m.root.Mod == nil || target.Use() != "aarch64/macos" {
		return ""
	}
	return m.root.Mod.Platforms["macos"]
}

// programDir finds the program a build names by name alone: `vsc run
// tcp-echo` is the checkout's cmd/tcp-echo, as the folder a Go command is.
// A bare name is the program even beside a folder of that name -- remote
// has the library hub/ and the tool cmd/hub -- and `./hub` names the
// folder. "" where the name is not one.
func programDir(name string) string {
	if name == "" || filepath.Ext(name) != "" || strings.ContainsAny(name, `/\`) {
		return ""
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, ok := importer.Enclosing(wd)
	if !ok {
		return ""
	}
	dir := filepath.Join(root.Dir, "cmd", name)
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

// programs are the checkout's cmd/* folders, for a message naming them.
func programs() []string {
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}
	root, ok := importer.Enclosing(wd)
	if !ok {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root.Dir, "cmd"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}
