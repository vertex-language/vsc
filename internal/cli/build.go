package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vertex-language/ir"
	irtext "github.com/vertex-language/ir/text"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/iface"
	"github.com/vertex-language/vsc/internal/sil/text"
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
	fs.StringVar(&b.packagePath, "package-path", "", "build the SwiftPM package rooted here (default: this directory, when it has a Package.swift)")
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
//
// What is built is, in order: the SwiftPM package here when there is a
// Package.swift and no files are named (see package.go); the program a
// bare name is, cmd/<name> of this checkout; the folder or files named;
// and with nothing named, the folder here.
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
	if root, product, ok := isPackageBuild(bf, names); ok {
		return doPackageBuild(bf, mode, root, product, target, stdout, stderr)
	}
	if len(names) == 1 {
		if dir := programDir(names[0]); dir != "" {
			names = []string{dir}
		}
	}
	if len(names) == 0 && hasVertex(".") {
		names = []string{"."}
	}
	return doFilesBuild(bf, mode, names, target, stdout, stderr)
}

// hasVertex reports whether dir holds .vs files.
func hasVertex(dir string) bool {
	found, _ := filepath.Glob(filepath.Join(dir, "*"+vsc.SourceExtension))
	return len(found) > 0
}

// doFilesBuild compiles the named files as one program or module, with what
// they import, and writes what --emit asks for.
func doFilesBuild(bf *buildFlags, mode emitMode, names []string, target ir.Target, stdout, stderr io.Writer) (string, int) {
	names, err := expandDirs(names, target)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	srcs, err := sources(names, target)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}

	// The program's folder is its module's, and its C++ is more of it.
	progDir := ""
	if len(names) > 0 && !isStdout(names[0]) {
		progDir = filepath.Dir(names[0])
		for _, n := range names[1:] {
			if filepath.Dir(n) != progDir {
				progDir = ""
				break
			}
		}
	}
	wd, _ := os.Getwd()
	if progDir != "" {
		bf.mainModule(progDir)
		pkgName := bf.module
		if pkgName == vsc.EntryModule {
			pkgName = "main"
		}
		own, err := bf.bind(progDir, "", pkgName, false, target)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitDiags
		}
		srcs = append(srcs, own...)
	} else {
		bf.mainModule(wd)
	}

	opts := bf.options(target, mode.stop)
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
	minOS := bf.main.minOS(target)

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
		obj, err := build.Object(u.VIR, build.Options{MinOS: minOS})
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, obj, false)
	}

	// exe and lib: the objects, then the link.
	if mode.name == "lib" && target.Use() != "aarch64/android" {
		fmt.Fprintf(stderr, "vsc: --emit lib builds for aarch64-android only, not %s\n", target.Use())
		return "", exitUsage
	}
	obj, err := build.Object(u.VIR, build.Options{MinOS: minOS})
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	inputs := []build.Input{{Name: outputName(srcs, ".o"), Data: obj}}
	seen := map[string]bool{inputs[0].Name: true}
	var need []build.Linkage
	addNative := func(dir string) int {
		objs, n, err := bf.nativeObjects(dir)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return exitDiags
		}
		for _, o := range objs {
			if !seen[o.Name] {
				seen[o.Name] = true
				inputs = append(inputs, o)
			}
		}
		if len(objs) > 0 {
			need = append(need, n)
		}
		return exitOK
	}
	if progDir != "" {
		if code := addNative(progDir); code != exitOK {
			return "", code
		}
	}
	for _, p := range u.Packages {
		pobj, code := buildPackage(p, bf, target, minOS, stderr)
		if code != exitOK {
			return "", code
		}
		if !seen[p.Name+".o"] {
			seen[p.Name+".o"] = true
			inputs = append(inputs, build.Input{Name: p.Name + ".o", Data: pobj})
		}
		if p.Native {
			if code := addNative(p.Dir); code != exitOK {
				return "", code
			}
		}
	}

	// A program that imports gpu gets the gpu runtime unit, and no
	// other program does.
	if vsc.ImportsGPU(u.Packages) {
		rt, err := build.GPURuntime(target)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		inputs = append(inputs, rt)
	}

	link := build.LinkOptions{
		Target:       target,
		Entry:        bf.entry,
		Freestanding: bf.freestanding,
		Swift:        len(u.Info.SwiftModules) > 0,
		Shared:       mode.name == "lib",
		MinOS:        minOS,
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
func buildPackage(pkg vsc.Package, bf *buildFlags, target ir.Target, minOS string, stderr io.Writer) ([]byte, int) {
	opts := bf.options(target, vsc.All)
	opts.Module = pkg.Name
	u, diags := vsc.Compile(pkg.Sources, opts)
	if printDiags(stderr, diags) {
		return nil, exitDiags
	}
	obj, err := build.Object(u.VIR, build.Options{MinOS: minOS})
	if err != nil {
		fmt.Fprintf(stderr, "vsc: package %s (%s): %v\n", pkg.Name, pkg.Dir, err)
		return nil, exitUsage
	}
	return obj, exitOK
}

func write(name string, stdout, stderr io.Writer, data []byte, exec bool) int {
	if err := writeOut(name, stdout, data, exec); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	return exitOK
}
