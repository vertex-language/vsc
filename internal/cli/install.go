package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/importer"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/timing"
	"github.com/vertex-language/vsc/token"
)

// BinEnv is the variable that says where vsc install puts programs.
const BinEnv = "VERTEXBIN"

// binDir is where installed programs go: VERTEXBIN where it is set, and
// .vertex/bin in the home directory otherwise -- as go install uses GOBIN,
// then ~/go/bin.
func binDir() (string, error) {
	if dir := os.Getenv(BinEnv); dir != "" {
		return filepath.Abs(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no home directory to install into: set %s", BinEnv)
	}
	return filepath.Join(home, ".vertex", "bin"), nil
}

// cmdInstall builds programs and puts them in the bin directory, as go
// install does.
//
// A program is named as build names one: a bare name is this checkout's
// cmd/<name>, a path is that folder, and an import path --
// github.com/you/tool/cmd/tool, or time/cmd/stopwatch, with @ref to pin a
// branch or tag -- is fetched and built from the package cache. A folder
// with no Vertex of its own but a cmd/ folder stands for every program
// in it, so `vsc install` at a checkout's root installs all its tools.
func cmdInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var bf buildFlags
	bf.register(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if bf.emit != "exe" || bf.output != "" {
		fmt.Fprintln(stderr, "vsc: install builds programs into the bin directory; use vsc build for --emit and -o")
		return exitUsage
	}
	target, err := bf.resolve()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	bin, err := binDir()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	// A program for another machine goes beside this machine's, in a
	// folder of its target's name, as Go's cross-compiled installs do.
	if host := vsc.HostName(); bf.target != host {
		bin = filepath.Join(bin, bf.target)
	}

	names := fs.Args()
	if len(names) == 0 {
		names = []string{"."}
	}
	var progs []program
	for _, name := range names {
		found, err := bf.findPrograms(name)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return exitUsage
		}
		progs = append(progs, found...)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}

	code := exitOK
	for _, p := range progs {
		// Each program is its own build: its own main module, vs.mod and
		// natives, as if it were built alone.
		one := buildFlags{
			common:       common{target: bf.target, module: bf.module, include: bf.include, pkgs: bf.pkgs, offline: bf.offline, update: bf.update, replace: bf.replace},
			emit:         "exe",
			entry:        bf.entry,
			freestanding: bf.freestanding,
		}
		dest := vsc.ImageName(target, filepath.Join(bin, p.name))
		// Built beside where it goes and renamed into place, so that a
		// program running while it is reinstalled keeps its own image:
		// macOS kills a process whose signed pages change under it.
		tmp, err := os.CreateTemp(bin, "."+p.name+".tmp-*")
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return exitUsage
		}
		tmp.Close()
		one.output = tmp.Name()
		one.notice = func(msg string) { fmt.Fprintln(stderr, "vsc:", msg) }
		_, c := doFilesBuild(&one, emitExe(), []string{p.dir}, target, stdout, stderr)
		if c != exitOK {
			os.Remove(tmp.Name())
			code = maxCode(code, c)
			continue
		}
		// CreateTemp made it 0600, which writing it kept.
		if err := os.Chmod(tmp.Name(), 0o755); err != nil {
			os.Remove(tmp.Name())
			fmt.Fprintln(stderr, "vsc:", err)
			code = maxCode(code, exitUsage)
			continue
		}
		if err := os.Rename(tmp.Name(), dest); err != nil {
			os.Remove(tmp.Name())
			fmt.Fprintln(stderr, "vsc:", err)
			code = maxCode(code, exitUsage)
			continue
		}
		fmt.Fprintf(stderr, "vsc: installed %s\n", dest)
	}
	timing.Print(stderr)
	if code == exitOK && !onPath(bin) {
		fmt.Fprintf(stderr, "vsc: %s is not on PATH; add it to run what is installed by name\n", bin)
	}
	return code
}

// A program is one folder to build and the name it is installed as.
type program struct {
	dir, name string
}

func emitExe() emitMode {
	m, _ := lookupEmit("exe")
	return m
}

// findPrograms is what one install argument names.
func (bf *buildFlags) findPrograms(name string) ([]program, error) {
	// A bare name is this checkout's cmd/<name>, even beside a folder of
	// that name, as for build and run.
	if dir := programDir(name); dir != "" {
		return []program{{dir, name}}, nil
	}
	if info, err := os.Stat(name); err == nil && info.IsDir() {
		abs, err := filepath.Abs(name)
		if err != nil {
			return nil, err
		}
		return programsIn(abs, filepath.Base(abs))
	}
	if _, err := os.Stat(name); err == nil {
		return nil, fmt.Errorf("%s is a file: a program is a folder, or this checkout's cmd/<name>", name)
	}
	if strings.HasPrefix(name, ".") || filepath.IsAbs(name) {
		return nil, fmt.Errorf("no folder %s", name)
	}
	// An import path: fetched, at the ref after its @ if it has one.
	m, ok := importer.Lookup(name)
	if !ok {
		var have string
		if cmds := programs(); len(cmds) > 0 {
			have = " (this checkout's programs: " + strings.Join(cmds, ", ") + ")"
		}
		return nil, fmt.Errorf("no program %q: not a cmd/ folder here, a folder, or an import path%s", name, have)
	}
	root, err := importer.Fetch(m, importer.Options{Offline: bf.offline, Update: bf.update, Log: bf.notice})
	if err != nil {
		return nil, err
	}
	dir := root
	if sub := m.Subdir(); sub != "" {
		dir = filepath.Join(root, filepath.FromSlash(sub))
	}
	return programsIn(dir, filepath.Base(filepath.FromSlash(m.Path)))
}

// programsIn is the program dir is, named name; or, where dir is not a
// program -- no Vertex of its own, or a library's -- every program in its
// cmd/ folder.
func programsIn(dir, name string) ([]program, error) {
	clause := ""
	if hasVertex(dir) {
		clause = packageClause(dir)
		if clause == "" || clause == "main" {
			return []program{{dir, name}}, nil
		}
	}
	// The programs are in its cmd/ folder -- or it is that folder.
	cmds := filepath.Join(dir, "cmd")
	if filepath.Base(dir) == "cmd" && clause == "" {
		cmds = dir
	}
	entries, err := os.ReadDir(cmds)
	if err != nil {
		if clause != "" {
			return nil, fmt.Errorf("%s is package %s, a library, and has no cmd/ folder of programs", dir, clause)
		}
		return nil, fmt.Errorf("%s: no %s files, and no cmd/ folder of programs", dir, vsc.SourceExtension)
	}
	var out []program
	for _, e := range entries {
		sub := filepath.Join(cmds, e.Name())
		if e.IsDir() && hasVertex(sub) && (packageClause(sub) == "" || packageClause(sub) == "main") {
			out = append(out, program{sub, e.Name()})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: cmd/ has no programs in it", dir)
	}
	return out, nil
}

// onPath reports whether dir is one of PATH's directories.
func onPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p != "" && filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

// packageClause is the name the package clause of dir's files gives, ""
// where none has one: a program's folder says main, or nothing at all.
func packageClause(dir string) string {
	files := vsc.SourceFiles(dir)
	for _, name := range files {
		text, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		f, _ := parser.ParseFile(token.NewFile(name, text), parser.SkipBodies|parser.Tolerant)
		if f == nil {
			continue
		}
		for _, s := range f.Stmts {
			if ds, ok := s.(*ast.DeclStmt); ok {
				if pd, ok := ds.D.(*ast.PackageDecl); ok && pd.Name != nil {
					return pd.Name.Text(f.Unit)
				}
			}
		}
	}
	return ""
}
