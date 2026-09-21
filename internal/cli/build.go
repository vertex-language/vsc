package cli

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/vertex-language/vsc/iface"
	"io"

	"github.com/vertex-language/ir"
	irtext "github.com/vertex-language/ir/text"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/internal/sil/text"
	"github.com/vertex-language/vsc/pkg"
	"github.com/vertex-language/vsc/token"
	"path/filepath"
)

// emitMode describes how far down the compilation pipeline to go and what to write.
type emitMode struct {
	name string
	stop vsc.Phase
	ext  string
}

var emits = []emitMode{
	{"exe", vsc.All, ""},
	{"obj", vsc.All, ".o"},
	// A shared library, for an Android app's NativeActivity to load.
	{"lib", vsc.All, ".so"},
	{"vir", vsc.All, ".vir"},
	{"sil", vsc.Lowered, ".sil"},
	// The SIL as generated, before the ownership passes verify it: for
	// reading what the generator wrote when the verifier refuses it.
	{"rawsil", vsc.Raw, ".sil"},
	{"interface", vsc.Checked, iface.Extension},
}

func lookupEmit(name string) (emitMode, bool) {
	for _, e := range emits {
		if e.name == name {
			return e, true
		}
	}
	return emitMode{}, false
}

func emitNames() string {
	var b bytes.Buffer
	for i, e := range emits {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(e.name)
	}
	return b.String()
}

// buildFlags holds flags for build and run.
type buildFlags struct {
	common
	emit         string
	output       string
	entry        string
	freestanding bool
	packagePath  string
}

func (b *buildFlags) register(fs *flag.FlagSet) {
	b.common.register(fs)
	fs.StringVar(&b.emit, "emit", "exe", "what to produce: "+emitNames())
	fs.StringVar(&b.output, "o", "", "write output here (\"-\" is standard output)")
	fs.StringVar(&b.entry, "entry", "", "the program's entry symbol (default: the platform's)")
	fs.BoolVar(&b.freestanding, "freestanding", false, "link no platform libraries")
	fs.StringVar(&b.packagePath, "package-path", "", "build the package rooted here (default: this directory, when it has a Package.swift)")
}

func cmdBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var bf buildFlags
	bf.register(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	_, code := doBuild(&bf, fs.Args(), stdout, stderr)
	return code
}

// doBuild compiles the sources and returns the output path and exit code.
func doBuild(bf *buildFlags, names []string, stdout, stderr io.Writer) (string, int) {
	mode, ok := lookupEmit(bf.emit)
	if !ok {
		fmt.Fprintf(stderr, "vsc: unknown --emit %q (known: %s)\n", bf.emit, emitNames())
		return "", exitUsage
	}
	target, err := bf.resolve()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	bf.notice = func(msg string) { fmt.Fprintln(stderr, "vsc:", msg) }
	// No files, in a package: the package. See package.go.
	if root, product, ok := isPackageBuild(bf, names); ok {
		return doPackageBuild(bf, mode, root, product, target, stdout, stderr)
	}
	return doFilesBuild(bf, mode, names, target, stdout, stderr)
}

// doFilesBuild compiles the named files as one program or module, with what
// they import, and writes what --emit asks for.
func doFilesBuild(bf *buildFlags, mode emitMode, names []string, target ir.Target, stdout, stderr io.Writer) (string, int) {
	names, err := expandDirs(names)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	srcs, err := sources(names)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}

	// Files that are a target of a package bring the C-family targets the
	// manifest has that target depend on: built first, so `import cwindow`
	// finds its interface, and linked with the program.
	var own *targetBuild
	if len(names) > 0 && !isStdout(names[0]) {
		own, err = bf.targets(filepath.Dir(names[0]), target)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
	}
	opts := bf.options(target, mode.stop)
	if own != nil && own.modules != "" {
		opts.ImportPaths = append([]string{own.modules}, opts.ImportPaths...)
	}

	u, diags := vsc.Compile(srcs, opts)
	if printDiags(stderr, diags) {
		return "", exitDiags
	}

	out := bf.output
	if out == "" {
		out = outputName(srcs, mode.ext)
	}
	if mode.name == "exe" {
		out = vsc.ImageName(target, out)
	}

	switch mode.name {
	case "interface":
		var buf bytes.Buffer
		if err := iface.Print(&buf, iface.Module{
			Name:  bf.module,
			Files: u.Files,
			Units: u.Positions,
			Info:  u.Info,
		}); err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, buf.Bytes(), false)

	case "sil", "rawsil":
		var buf bytes.Buffer
		if err := text.Print(&buf, u.SIL); err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, buf.Bytes(), false)

	case "vir":
		var buf bytes.Buffer
		if err := irtext.Print(&buf, u.VIR); err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, buf.Bytes(), false)

	case "obj":
		obj, err := object(u.VIR)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, obj, false)
	}

	// exe and lib: the object, then the link.
	if mode.name == "lib" && target.Use() != "aarch64/android" {
		fmt.Fprintf(stderr, "vsc: --emit lib builds for aarch64-android only, not %s\n", target.Use())
		return "", exitUsage
	}
	obj, err := object(u.VIR)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	// Build program object and any imported package dependencies, then link.
	inputs := []build.Input{{Name: outputName(srcs, ".o"), Data: obj}}
	var libs []string
	var need []build.Linkage
	seenInput := map[string]bool{}
	for _, in := range inputs {
		seenInput[in.Name] = true
	}
	if own != nil {
		for _, obj := range own.objs {
			if !seenInput[obj.Name] {
				seenInput[obj.Name] = true
				inputs = append(inputs, obj)
			}
		}
		need = append(need, own.need)
	}
	for _, p := range u.Packages {
		pobj, code := buildPackage(p, bf, target, stderr)
		if code != exitOK {
			return "", code
		}
		if !seenInput[p.Name+".o"] {
			seenInput[p.Name+".o"] = true
			inputs = append(inputs, build.Input{Name: p.Name + ".o", Data: pobj})
		}
		// A folder that is a target of a package brings the C-family
		// targets it depends on, which its manifest says how to build.
		built, err := bf.targets(p.Dir, target)
		if err != nil {
			fmt.Fprintf(stderr, "vsc: package %s (%s): %v\n", p.Name, p.Dir, err)
			return "", exitUsage
		}
		if built != nil {
			for _, obj := range built.objs {
				if !seenInput[obj.Name] {
					seenInput[obj.Name] = true
					inputs = append(inputs, obj)
				}
			}
			need = append(need, built.need)
		}
	}

	link := build.LinkOptions{
		Target:       target,
		Entry:        bf.entry,
		Freestanding: bf.freestanding,
		Swift:        len(u.Info.SwiftModules) > 0,
		LibNames:     libs,
		Shared:       mode.name == "lib",
	}
	if link.Shared {
		link.SOName = filepath.Base(out)
	}
	for _, n := range need {
		link = n.Options(link)
	}
	exe, err := build.Executable(inputs, link)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	return out, write(out, stdout, stderr, exe, true)
}

// buildPackage compiles one imported folder as its own module.
func buildPackage(pkg vsc.Package, bf *buildFlags, target ir.Target, stderr io.Writer) ([]byte, int) {
	opts := bf.options(target, vsc.All)
	opts.Module = pkg.Name
	opts.ImportPaths = append(opts.ImportPaths, pkg.ImportPaths...)
	u, diags := vsc.Compile(pkg.Sources, opts)
	if printDiags(stderr, diags) {
		return nil, exitDiags
	}
	obj, err := object(u.VIR)
	if err != nil {
		fmt.Fprintf(stderr, "vsc: package %s (%s): %v\n", pkg.Name, pkg.Dir, err)
		return nil, exitUsage
	}
	return obj, exitOK
}

func object(m *ir.Module) ([]byte, error) { return build.Object(m, build.Options{}) }

func write(name string, stdout, stderr io.Writer, data []byte, exec bool) int {
	if err := writeOut(name, stdout, data, exec); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	return exitOK
}

// A targetBuild is what a package target's C-family dependencies built to.
type targetBuild struct {
	objs    []build.Input
	need    build.Linkage
	modules string
}

// targets finds the manifest of the package a folder is a target of,
// walking up from the folder, and compiles the C-family targets that target
// depends on, once. A folder no manifest claims brings nothing, and is nil.
func (c *common) targets(dir string, target ir.Target) (*targetBuild, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if b, ok := c.built[abs]; ok {
		return b, nil
	}
	b, err := packageTargets(abs, target)
	if err != nil {
		return nil, err
	}
	if c.built == nil {
		c.built = map[string]*targetBuild{}
	}
	c.built[abs] = b
	return b, nil
}

func packageTargets(abs string, target ir.Target) (*targetBuild, error) {
	for d := abs; ; d = filepath.Dir(d) {
		if _, ok := pkg.FindManifest(d); ok {
			m, diags, err := pkg.Load(d)
			if err != nil {
				return nil, err
			}
			for _, diag := range diags {
				if diag.Severity == token.Error {
					return nil, fmt.Errorf("%s: %s", d, diag.Message)
				}
			}
			platform := pkg.PlatformOf(target.Use())
			p, err := pkg.Resolve(d, m, platform, "debug")
			if err != nil {
				return nil, err
			}
			for _, t := range p.Targets {
				if filepath.Clean(t.Dir) == abs {
					work := filepath.Join(d, ".build", "vsc", "debug")
					objs, need, err := build.TargetObjects(p, t, build.PackageOptions{Target: target, Config: "debug", Work: work})
					if err != nil {
						return nil, err
					}
					return &targetBuild{objs: objs, need: need, modules: filepath.Join(work, "Modules")}, nil
				}
			}
			return nil, nil
		}
		if filepath.Dir(d) == d {
			return nil, nil
		}
	}
}
