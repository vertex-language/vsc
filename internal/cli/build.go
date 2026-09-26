package cli

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/vertex-language/vsc/timing"
	"io"
	"os"
	"path/filepath"

	"github.com/vertex-language/ir"
	irtext "github.com/vertex-language/ir/text"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/build/buildcache"
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
	timing.Print(stderr)
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
	// A program linked whole may have its own object cached: then the
	// compile stops once its imports are known, which is what the link
	// needs of it.
	var mainKey buildcache.Key
	var cachedMain []byte
	keyedMain := false
	// A program linked whole has its packages built as the importer finds
	// them; see packageStream.
	var stream *packageStream
	if mode.name == "exe" || mode.name == "lib" {
		minOS := bf.main.minOS(target)
		streamDir := progDir
		if streamDir == "" {
			streamDir = wd
		}
		stream = newPackageStream(&bf.common, bf, target, streamDir, minOS, stderr)
		opts.OnPackage = stream.add
		opts.AfterImports = func(pkgs []vsc.Package, opaque bool) bool {
			if opaque {
				return true
			}
			mainKey, keyedMain = programKey(srcs, bf.module, target, minOS, pkgs)
			if !keyedMain {
				return true
			}
			if entry, hit := buildcache.Get(mainKey); hit {
				if warnings, obj, ok := splitMain(entry); ok {
					timing.Count("cache hit: main", 1)
					stderr.Write(warnings)
					cachedMain = obj
					return false
				}
			}
			timing.Count("cache miss: main", 1)
			return true
		}
	}
	u, diags := vsc.Compile(srcs, opts)
	// A program taken from the cache has had its warnings, the imports'
	// among them, printed from the cache already.
	var warnings bytes.Buffer
	if cachedMain != nil && !vsc.Errors(diags) {
		diags = nil
	}
	if printDiags(io.MultiWriter(stderr, &warnings), diags) {
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
	obj := cachedMain
	if obj == nil {
		var err error
		obj, err = build.Object(u.VIR, build.Options{MinOS: minOS})
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		if keyedMain {
			buildcache.Put(mainKey, joinMain(warnings.Bytes(), obj))
		}
	}
	inputs := []build.Input{{Name: outputName(srcs, ".o"), Data: obj}}
	seen := map[string]bool{inputs[0].Name: true}
	var need []build.Linkage
	// early holds native objects compiled ahead, while the packages were.
	early := map[string]nativeResult{}
	addNative := func(dir string) int {
		r, ok := early[dir]
		if !ok {
			r.objs, r.need, r.err = bf.nativeObjects(dir)
		}
		objs, n, err := r.objs, r.need, r.err
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
	mainDir := progDir
	if mainDir == "" {
		mainDir = wd
	}
	// The packages' C++ does not depend on their Vertex, so where the
	// packages compile in worker processes it compiles here meanwhile.
	// In-process package builds share the bound natives with it, and
	// run after as before.
	nativeDone := func() {}
	if jobs() > 1 {
		var dirs []string
		for _, p := range u.Packages {
			if p.Native {
				dirs = append(dirs, p.Dir)
			}
		}
		if len(dirs) > 0 {
			var results []nativeResult
			done := make(chan struct{})
			go func() {
				defer close(done)
				results = bf.nativeObjectsAll(dirs)
			}()
			nativeDone = func() {
				<-done
				for i, d := range dirs {
					early[d] = results[i]
				}
			}
		}
	}
	var pobjs [][]byte
	var code int
	if stream != nil {
		pobjs, code = stream.finish(u.Packages)
	} else {
		pobjs, code = packageObjects(u.Packages, bf, target, mainDir, minOS, stderr)
	}
	nativeDone()
	if code != exitOK {
		return "", code
	}
	for i, p := range u.Packages {
		pobj := pobjs[i]
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
		Swift:        u.Info != nil && len(u.Info.SwiftModules) > 0,
		Shared:       mode.name == "lib",
		MinOS:        minOS,
	}
	if link.Shared {
		link.SOName = filepath.Base(out)
	}
	for _, n := range need {
		link = n.Options(link)
	}
	lk, keyedLink := linkKey(inputs, link)
	exe, hit := []byte(nil), false
	if keyedLink {
		exe, hit = buildcache.Get(lk)
	}
	if hit {
		timing.Count("cache hit: link", 1)
	} else {
		var err error
		exe, err = build.Executable(inputs, link)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		if keyedLink {
			buildcache.Put(lk, exe)
		}
	}
	return out, write(out, stdout, stderr, exe, true)
}

// programKey is what the program's own object is cached under: its
// sources, the module it is compiled as, and the surface of every package
// it imports (see packageKey). ok is false where a package has no key.
func programKey(srcs []vsc.Source, module string, target ir.Target, minOS string, pkgs []vsc.Package) (buildcache.Key, bool) {
	if !buildcache.Enabled() {
		return buildcache.Key{}, false
	}
	surfaces := map[string]buildcache.Key{}
	h := buildcache.New("program").String(target.String()).String(minOS).String(module).
		String(fmt.Sprint(len(srcs)))
	for _, s := range srcs {
		h.String(s.Name).Bytes(s.Text)
	}
	h.String(fmt.Sprint(len(pkgs)))
	for _, p := range pkgs {
		if p.Opaque {
			return buildcache.Key{}, false
		}
		for _, d := range p.Deps {
			if _, ok := surfaces[d]; !ok {
				return buildcache.Key{}, false
			}
		}
		k := surfaceKey(p, surfaces)
		surfaces[p.Dir] = k
		h.String(p.Dir).Bytes(k[:])
	}
	return h.Key(), true
}

// A cached program object is stored with the warnings its compile gave,
// which a build that takes it from the cache prints again.
func joinMain(warnings, obj []byte) []byte {
	out := binary.AppendUvarint(nil, uint64(len(warnings)))
	out = append(out, warnings...)
	return append(out, obj...)
}

func splitMain(entry []byte) (warnings, obj []byte, ok bool) {
	n, w := binary.Uvarint(entry)
	if w <= 0 || uint64(len(entry)-w) < n {
		return nil, nil, false
	}
	return entry[w : w+int(n)], entry[w+int(n):], true
}

// linkKey is what a linked image is cached under: every input, every
// option, and the SDK its stubs are read from. On macOS a named library
// is found in the SDK, whose settings the key hashes; a link that searches
// directories of its own, or links for anything else, has no key, since
// what it reads there may change under it.
func linkKey(inputs []build.Input, link build.LinkOptions) (buildcache.Key, bool) {
	if !buildcache.Enabled() || link.Target.Use() != "aarch64/macos" ||
		len(link.LibDirs) > 0 || len(link.FrameworkDirs) > 0 {
		return buildcache.Key{}, false
	}
	h := buildcache.New("link").String(link.Target.String()).String(link.Entry).String(link.MinOS).
		String(link.SDK).Strings(link.Frameworks).Strings(link.LibNames).
		Strings(build.LibraryDirs(link.Target, link.Freestanding)).
		String(fmt.Sprint(link.Swift, link.NoRuntime, link.Freestanding, link.Shared)).String(link.SOName)
	sdk := link.SDK
	if sdk == "" {
		sdk, _ = build.SDK()
	}
	h.String(sdk)
	if settings, err := os.ReadFile(filepath.Join(sdk, "SDKSettings.json")); err == nil {
		h.Bytes(settings)
	}
	h.String(fmt.Sprint(len(link.Libs)))
	for _, in := range link.Libs {
		h.String(in.Name).Bytes(in.Data)
	}
	h.String(fmt.Sprint(len(inputs)))
	for _, in := range inputs {
		h.String(in.Name).Bytes(in.Data)
	}
	return h.Key(), true
}

// packageObjects is the object of every imported package, by index:
// from the build cache where it has one, and compiled -- in parallel
// where there are several -- where it does not.
//
// A package's object depends on the compiler, the target, its own
// sources, and the packages it imports -- it specializes their generics
// and inlines their bodies -- so its key is those, with each import's
// own key standing for the import. pkgs lists an import before anything
// that imports it, so every key can be made before anything compiles. A
// package with no key (an import from an interface file somewhere under
// it) is compiled every time.
func packageObjects(pkgs []vsc.Package, bf *buildFlags, target ir.Target, mainDir, minOS string, stderr io.Writer) ([][]byte, int) {
	objs := make([][]byte, len(pkgs))
	keys := map[string]buildcache.Key{}
	surfaces := map[string]buildcache.Key{}
	keyed := make([]bool, len(pkgs))
	var miss []int
	for i, p := range pkgs {
		key, ok := packageKey(p, target, minOS, surfaces)
		if ok {
			keys[p.Dir] = key
			surfaces[p.Dir] = surfaceKey(p, surfaces)
			keyed[i] = true
			if obj, hit := buildcache.Get(key); hit {
				timing.Count("cache hit: package", 1)
				objs[i] = obj
				continue
			}
		}
		miss = append(miss, i)
	}
	if len(miss) == 0 {
		return objs, exitOK
	}
	timing.Count("cache miss: package", len(miss))
	built, code := compilePackages(pkgs, miss, bf, mainDir, minOS, stderr)
	if code != exitOK {
		return nil, code
	}
	for _, i := range miss {
		objs[i] = built[i]
		if keyed[i] {
			buildcache.Put(keys[pkgs[i].Dir], built[i])
		}
	}
	return objs, exitOK
}

// packageKey is what a package's object is cached under: the package's own
// source, and what its compile can depend on of each package it imports --
// their surfaces, not their sources. An edit to a body no client compiles
// changes the edited package's key and no other's: early cutoff.
func packageKey(pkg vsc.Package, target ir.Target, minOS string, surfaces map[string]buildcache.Key) (buildcache.Key, bool) {
	if pkg.Opaque || !buildcache.Enabled() {
		return buildcache.Key{}, false
	}
	h := buildcache.New("package").
		String(target.String()).
		String(minOS).
		String(pkg.Name).
		String(fmt.Sprint(len(pkg.Sources)))
	for _, s := range pkg.Sources {
		h.String(s.Name).Bytes(s.Text)
	}
	h.String(fmt.Sprint(len(pkg.Deps)))
	for _, d := range pkg.Deps {
		k, ok := surfaces[d]
		if !ok {
			return buildcache.Key{}, false
		}
		h.Bytes(k[:])
	}
	return h.Key(), true
}

// surfaceKey is what a client's compile can depend on of pkg: its surface
// and, since a client reads them too, those of everything it imports.
// pkg's imports come before it, and are in surfaces already.
func surfaceKey(pkg vsc.Package, surfaces map[string]buildcache.Key) buildcache.Key {
	h := buildcache.New("surface").Bytes(pkg.Surface[:]).String(fmt.Sprint(len(pkg.Deps)))
	for _, d := range pkg.Deps {
		k := surfaces[d]
		h.Bytes(k[:])
	}
	return h.Key()
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

// nativeResult is what nativeObjects returned for one folder.
type nativeResult struct {
	objs []build.Input
	need build.Linkage
	err  error
}
