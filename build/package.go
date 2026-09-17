package build

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/macho"
	"github.com/vertex-language/objv"
	objvpp "github.com/vertex-language/objv/preprocessor"
	objvsys "github.com/vertex-language/objv/sysroot"
	"github.com/vertex-language/vcc"
	vccpp "github.com/vertex-language/vcc/preprocessor"
	"github.com/vertex-language/vcx"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/iface"
	"github.com/vertex-language/vsc/pkg"
)

// PackageOptions configures building targets from a package manifest.
type PackageOptions struct {
	Target ir.Target
	// Config is "debug" or "release": which conditional settings apply.
	Config string
	// Work is where objects and interfaces are written.
	Work string
}

// A Product is a linked program.
type Product struct {
	Name  string
	Image []byte
}

// A PackageError is a target that did not build.
type PackageError struct {
	Target string
	// Diags are the compiler's, for a Swift target it refused.
	Diags []vsc.Diagnostic
	Err   error
}

func (e *PackageError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("target '%s': %v", e.Target, e.Err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "target '%s' did not compile", e.Target)
	for _, d := range e.Diags {
		b.WriteString("\n")
		b.WriteString(d.String())
	}
	return b.String()
}

// BuildPackage builds every target of a laid-out package and links each of
// its programs.
func BuildPackage(p *pkg.Package, opts PackageOptions) ([]Product, error) {
	if opts.Config == "" {
		opts.Config = "debug"
	}
	modules := filepath.Join(opts.Work, "Modules")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		return nil, err
	}
	b := &packageBuild{p: p, opts: opts, modules: modules, minOS: deploymentTarget(p.Manifest, opts.Target)}
	objs := map[*pkg.ResolvedTarget][]Input{}
	for _, t := range p.Targets {
		if t.Kind == pkg.TargetTest {
			continue
		}
		var err error
		if t.Swift() {
			objs[t], err = b.swiftTarget(t)
		} else {
			objs[t], err = b.cTarget(t)
		}
		if err != nil {
			return nil, err
		}
	}

	var out []Product
	for _, exe := range p.Executables() {
		var inputs []Input
		var need Linkage
		for _, t := range exe.Target.Closure() {
			inputs = append(inputs, objs[t]...)
			need.add(t)
		}
		image, err := Executable(inputs, need.options(LinkOptions{Target: opts.Target, MinOS: b.minOS}))
		if err != nil {
			return nil, &PackageError{Target: exe.Target.Name, Err: err}
		}
		out = append(out, Product{Name: exe.Name, Image: image})
	}
	return out, nil
}

// Linkage is what the targets a program is built from need at the link,
// beyond their objects: the libraries and frameworks they name, and the
// language runtimes their sources imply.
type Linkage struct {
	Libraries  []string
	Frameworks []string
	CXX        bool
	ObjC       bool
	// MinOS is the deployment target the package's manifest declares,
	// which its objects are built for and a program linking them is held
	// to; empty where it declares none.
	MinOS string
}

func (n *Linkage) add(t *pkg.ResolvedTarget) {
	n.Libraries = appendNew(n.Libraries, t.Libraries...)
	n.Frameworks = appendNew(n.Frameworks, t.Frameworks...)
	n.CXX = n.CXX || t.UsesCXX()
	for _, s := range t.Sources {
		n.ObjC = n.ObjC || s.Language == pkg.ObjC
	}
}

// options adds what n needs to a link's options.
//
// A program with C++ in it needs the C++ library, and one with Objective-C
// the runtime, which SwiftPM links without being asked.
func (n Linkage) options(opts LinkOptions) LinkOptions {
	opts.LibNames = appendNew(opts.LibNames, n.Libraries...)
	opts.Frameworks = appendNew(opts.Frameworks, n.Frameworks...)
	if opts.Target.Use() == "aarch64/macos" {
		if n.CXX {
			opts.LibNames = appendNew(opts.LibNames, "c++")
		}
		if n.ObjC {
			opts.LibNames = appendNew(opts.LibNames, "objc")
		}
	}
	opts.MinOS = laterOS(opts.MinOS, n.MinOS)
	return opts
}

// laterOS is the later of two deployment targets, either possibly empty.
// A program is built for the latest one anything it links was built for,
// as a SwiftPM package is for the highest of its dependencies' platforms.
func laterOS(a, b string) string {
	if a == "" {
		a = defaultMinOS
	}
	if b == "" {
		return a
	}
	va, errA := macho.ParseVersion(a)
	vb, errB := macho.ParseVersion(b)
	if errA != nil || errB != nil || vb <= va {
		return a
	}
	return b
}

// Options adds what n needs to a link's options.
func (n Linkage) Options(opts LinkOptions) LinkOptions { return n.options(opts) }

// TargetObjects compiles the C-family targets a target of p depends on --
// not the target itself, which is Vertex and compiled as a module of its
// own -- and says what the link needs for them.
func TargetObjects(p *pkg.Package, t *pkg.ResolvedTarget, opts PackageOptions) ([]Input, Linkage, error) {
	var need Linkage
	if opts.Config == "" {
		opts.Config = "debug"
	}
	modules := filepath.Join(opts.Work, "Modules")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		return nil, need, err
	}
	b := &packageBuild{p: p, opts: opts, modules: modules, minOS: deploymentTarget(p.Manifest, opts.Target)}
	need.MinOS = b.minOS
	var objs []Input
	for _, d := range t.Closure() {
		if d.Kind == pkg.TargetTest {
			continue
		}
		// The target itself is Vertex and compiled as a module of its
		// own, but what it links against is still the program's.
		need.add(d)
		if d == t || d.Swift() {
			continue
		}
		got, err := b.cTarget(d)
		if err != nil {
			return nil, need, err
		}
		objs = append(objs, got...)
	}
	return objs, need, nil
}

type packageBuild struct {
	p       *pkg.Package
	opts    PackageOptions
	modules string
	minOS   string
}

// swiftTarget compiles a Swift target as a module named for it, or as the
// program's entry module where it is an executable.
func (b *packageBuild) swiftTarget(t *pkg.ResolvedTarget) ([]Input, error) {
	var srcs []vsc.Source
	for _, s := range t.Sources {
		text, err := os.ReadFile(s.Path)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, vsc.Source{Name: s.Path, Text: text})
	}
	module := t.Name
	if t.Kind == pkg.TargetExecutable {
		module = vsc.EntryModule
	}
	unit, diags := vsc.Compile(srcs, vsc.Options{Module: module, Target: b.opts.Target, ImportPaths: []string{b.modules}})
	if vsc.Errors(diags) {
		return nil, &PackageError{Target: t.Name, Diags: diags}
	}
	obj, err := Object(unit.VIR, Options{MinOS: b.minOS})
	if err != nil {
		return nil, &PackageError{Target: t.Name, Err: err}
	}
	if t.Kind != pkg.TargetExecutable {
		var buf bytes.Buffer
		if err := iface.Print(&buf, iface.Module{Name: t.Name, Files: unit.Files, Units: unit.Positions, Info: unit.Info}); err != nil {
			return nil, &PackageError{Target: t.Name, Err: err}
		}
		if err := os.WriteFile(filepath.Join(b.modules, t.Name+iface.Extension), buf.Bytes(), 0o644); err != nil {
			return nil, err
		}
	}
	return []Input{{Name: t.Name + ".o", Data: obj}}, nil
}

// cTarget compiles a C, C++ or Objective-C target, and writes the interface
// its public headers give the Swift targets that import it.
//
// Nothing here runs a host toolchain. C is vcc's, C++ is vcx's and
// Objective-C is objv's, each called as a library in this process, and
// what comes back is object bytes for the same linker the Swift targets
// feed. A machine with Go and an SDK's headers builds a package; a machine
// with clang does not build it any differently.
func (b *packageBuild) cTarget(t *pkg.ResolvedTarget) ([]Input, error) {
	fl, err := b.cFlags(t)
	if err != nil {
		return nil, &PackageError{Target: t.Name, Err: err}
	}
	cc := cCompilers{b: b, t: t, flags: fl}

	var objs []Input
	for _, src := range t.Sources {
		rel, _ := filepath.Rel(t.Dir, src.Path)
		data, err := cc.object(src)
		if err != nil {
			return nil, &PackageError{Target: t.Name, Err: fmt.Errorf("%s: %w", rel, err)}
		}
		name := t.Name + "_" + strings.ReplaceAll(rel, string(filepath.Separator), "_") + ".o"
		objs = append(objs, Input{Name: name, Data: data})
	}

	if t.PublicHeaders != "" {
		text, _, err := CInterface(CImport{
			Module:      t.Name,
			Headers:     t.PublicHeaders,
			IncludeDirs: fl.includes,
			Defines:     append(append([]string(nil), fl.defines["c"]...), fl.defines["cxx"]...),
			CXX:         t.UsesCXX(),
			Target:      b.opts.Target,
		})
		if err != nil {
			return nil, &PackageError{Target: t.Name, Err: err}
		}
		if err := os.WriteFile(filepath.Join(b.modules, t.Name+iface.Extension), text, 0o644); err != nil {
			return nil, err
		}
	}
	return objs, nil
}

// cFlags are what a C-family target's manifest settings say, per tool, in
// the terms the in-tree compilers take.
type cFlags struct {
	includes []string
	// defines and undefines are NAME or NAME=VALUE, by tool ("c", "cxx").
	defines   map[string][]string
	undefines map[string][]string
}

// cFlags collects a target's defines and include directories.
//
// unsafeFlags were clang's command line, and only the part of it that says
// something a compiler here can hear is kept: -D, -U and -I. Anything else is
// refused by name rather than dropped, because a flag that changed what
// clang built and changes nothing here is a build that differs silently.
func (b *packageBuild) cFlags(t *pkg.ResolvedTarget) (cFlags, error) {
	fl := cFlags{
		includes:  t.IncludeDirs(),
		defines:   map[string][]string{},
		undefines: map[string][]string{},
	}
	for _, tool := range []string{"c", "cxx"} {
		fl.defines[tool] = append(fl.defines[tool], "SWIFT_PACKAGE=1")
		if b.opts.Config == "debug" {
			fl.defines[tool] = append(fl.defines[tool], "DEBUG=1")
		}
		fl.defines[tool] = append(fl.defines[tool], t.Defines[tool]...)

		flags := t.UnsafeFlags[tool]
		for i := 0; i < len(flags); i++ {
			f := flags[i]
			var opt, val string
			switch {
			case f == "-D" || f == "-U" || f == "-I":
				if i+1 == len(flags) {
					return fl, fmt.Errorf("unsafeFlags: %s needs a value", f)
				}
				opt, val = f, flags[i+1]
				i++
			case strings.HasPrefix(f, "-D"), strings.HasPrefix(f, "-U"), strings.HasPrefix(f, "-I"):
				opt, val = f[:2], f[2:]
			case strings.HasPrefix(f, "-O"), strings.HasPrefix(f, "-g"), strings.HasPrefix(f, "-W"):
				// Optimization, debug info and warning levels ask nothing of
				// what the program means.
				continue
			default:
				return fl, fmt.Errorf("unsafeFlags: %q is a flag for clang, and no compiler here takes it", f)
			}
			switch opt {
			case "-D":
				fl.defines[tool] = append(fl.defines[tool], val)
			case "-U":
				fl.undefines[tool] = append(fl.undefines[tool], val)
			case "-I":
				if !filepath.IsAbs(val) {
					val = filepath.Join(t.Dir, val)
				}
				fl.includes = append(fl.includes, val)
			}
		}
	}
	return fl, nil
}

// cCompilers holds one compiler per language for one target, made on first
// use: each resolves its search list and predefines once, and reuses them
// for every file after.
type cCompilers struct {
	b     *packageBuild
	t     *pkg.ResolvedTarget
	flags cFlags

	c    *vcc.Compiler
	cxx  *vcx.Compiler
	objc *objv.Compiler
}

// object compiles one source file to object bytes.
func (cc *cCompilers) object(src pkg.SourceFile) ([]byte, error) {
	name := targetName(cc.b.opts.Target)
	switch src.Language {
	case pkg.C:
		if cc.c == nil {
			cc.c = &vcc.Compiler{
				Target:      name,
				IncludeDirs: cc.flags.includes,
				Defines:     vccPredefines(cc.flags.defines["c"], cc.flags.undefines["c"]),
			}
		}
		data, diags, err := cc.c.Object(vcc.File(src.Path))
		if err == nil && vcc.HasErrors(diags) {
			err = &vcc.DiagnosticError{Diagnostics: diags}
		}
		return data, err

	case pkg.CXX:
		if cc.cxx == nil {
			cc.cxx = &vcx.Compiler{
				Target:      name,
				Std:         cxxStd(cc.b.p.Manifest.CXXLanguageStandard),
				IncludeDirs: cc.flags.includes,
				Defs:        cc.flags.defines["cxx"],
				Undefs:      cc.flags.undefines["cxx"],
			}
		}
		data, diags, err := cc.cxx.Object(vcx.File(src.Path))
		if err == nil && vcx.HasErrors(diags) {
			err = &vcx.DiagnosticError{Diagnostics: diags}
		}
		return data, err

	case pkg.ObjC:
		if cc.objc == nil {
			cc.objc = &objv.Compiler{
				Target:      name,
				IncludeDirs: cc.flags.includes,
				Defines:     objvPredefines(cc.flags.defines["c"], cc.flags.undefines["c"]),
				// SwiftPM compiles a target's Objective-C with ARC -- its
				// `swift build -v` passes -fobjc-arc -- and a manifest
				// means what SwiftPM does with it. Without it, a strong
				// static or an autoreleased object kept past its pool is
				// a dangling reference in code written for ARC.
				ARC: true,
			}
			if v, ok := objvsys.ParseVersion(cc.b.minOS); ok {
				cc.objc.Deployment = v
			}
		}
		data, diags, err := cc.objc.Object(objv.File(src.Path))
		if err == nil && objv.HasErrors(diags) {
			err = &objv.DiagnosticError{Diagnostics: diags}
		}
		return data, err

	case pkg.ObjCXX:
		return nil, errors.New("Objective-C++ has no compiler here yet: objv is Objective-C over C, and vcx is C++ without it")
	case pkg.Assembly:
		return nil, errors.New("assembly sources are not built yet")
	}
	return nil, fmt.Errorf("no compiler for this kind of source")
}

// vccPredefines is -D and then -U, in vcc's spelling.
func vccPredefines(defines, undefines []string) []vccpp.Predefine {
	out := make([]vccpp.Predefine, 0, len(defines)+len(undefines))
	for _, d := range defines {
		out = append(out, vccpp.Predefine{Kind: vccpp.PredefineDefine, Text: d})
	}
	for _, u := range undefines {
		out = append(out, vccpp.Predefine{Kind: vccpp.PredefineUndef, Text: u})
	}
	return out
}

// objvPredefines is -D and then -U, in objv's spelling.
func objvPredefines(defines, undefines []string) []objvpp.Predefine {
	out := make([]objvpp.Predefine, 0, len(defines)+len(undefines))
	for _, d := range defines {
		out = append(out, objvpp.Predefine{Kind: objvpp.PredefineDefine, Text: d})
	}
	for _, u := range undefines {
		out = append(out, objvpp.Predefine{Kind: objvpp.PredefineUndef, Text: u})
	}
	return out
}

// cxxStd is a manifest's cxxLanguageStandard as a vcx standard. vcx reads
// nothing older than C++20, so an older standard compiles as C++20, which
// accepts what those programs are written in.
func cxxStd(std string) vcx.Std {
	switch strings.TrimPrefix(strings.TrimPrefix(std, "gnu++"), "c++") {
	case "26", "2c":
		return vcx.Cxx26
	case "23", "2b", "":
		return vcx.Cxx23
	}
	return vcx.Cxx20
}

// deploymentTarget is the macOS version a package builds for: what its
// manifest names, or the version this compiler links for otherwise.
func deploymentTarget(m *pkg.Manifest, target ir.Target) string {
	if target.Use() == "aarch64/macos" {
		for _, p := range m.Platforms {
			if p.Name == "macos" && p.Version != "" {
				return p.Version
			}
		}
	}
	return defaultMinOS
}

func appendNew(list []string, items ...string) []string {
	for _, it := range items {
		found := false
		for _, have := range list {
			if have == it {
				found = true
				break
			}
		}
		if !found {
			list = append(list, it)
		}
	}
	return list
}
