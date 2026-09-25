package build

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/macho"
	macholink "github.com/vertex-language/macho/link"
	"github.com/vertex-language/pe"
	pelink "github.com/vertex-language/pe/link"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build/sysroot"
)

// ErrLink indicates a failure during the link step.
var ErrLink = errors.New("build: link")

// An Input represents an object, archive, or library stub passed to the linker.
type Input struct {
	Name string
	Data []byte
}

// LinkOptions configures executable linking.
type LinkOptions struct {
	// Target specifies the machine and container to link for.
	Target ir.Target

	// Entry is the entry point symbol. If empty, the platform default is used
	// (_main for Mach-O, mainCRTStartup for PE).
	Entry string

	// MinOS specifies the minimum deployment OS version for Mach-O images.
	MinOS string

	// SDK specifies the platform SDK directory.
	SDK string

	// LibDirs are directories searched for named libraries before platform defaults.
	LibDirs []string

	// LibNames are library names resolved against LibDirs and platform paths.
	LibNames []string

	// Frameworks are framework names, resolved against FrameworkDirs and
	// the SDK's frameworks: "Cocoa" links Cocoa.framework's stub. Mach-O
	// only, and linked before LibNames, so that a framework's re-exports
	// are what a library that also names them resolves to.
	Frameworks []string

	// FrameworkDirs are directories searched for frameworks before the SDK's.
	FrameworkDirs []string

	// Swift links the Swift runtime bridge and libswiftCore stub.
	Swift bool

	// NoRuntime omits linking the in-tree Vertex runtime.
	NoRuntime bool

	// Freestanding omits all platform SDK libraries.
	Freestanding bool

	// Libs are additional archives or stubs linked in order.
	Libs []Input

	// Shared links a shared library rather than a program: on Android,
	// the lib<name>.so an app's NativeActivity loads.
	Shared bool

	// SOName is a shared library's DT_SONAME.
	SOName string
}

// Executable links objects into a runnable image and returns its
// bytes.
//
// This is the step past Object, and the last one that is entirely the
// compiler's own: what comes out needs no toolchain to have been
// installed and no `ld` to have been run. The linker is
// vertex-language's, shared with vcx, and takes bytes and returns
// Executable links objects into a runnable image and returns its bytes.
func Executable(objs []Input, opts LinkOptions) ([]byte, error) {
	if len(objs) == 0 {
		return nil, fmt.Errorf("%w: nothing to link", ErrLink)
	}
	switch opts.Target.Use() {
	case "aarch64/macos":
		return aarch64MachOExe(objs, opts)
	case "x86_64/windows":
		return amd64PEExe(objs, opts)
	case "aarch64/android":
		return aarch64AndroidELF(objs, opts)
	}
	return nil, fmt.Errorf("%w: %s", ErrTarget, opts.Target.Use())
}

func aarch64MachOExe(objs []Input, opts LinkOptions) ([]byte, error) {
	target := macho.Target{
		CPU:      macho.CPU_TYPE_ARM64,
		SubCPU:   macho.CPU_SUBTYPE_ARM64_ALL,
		Platform: macho.PlatformMacOS,
		Endian:   macho.LittleEndian,
	}
	minOS := opts.MinOS
	if minOS == "" {
		minOS = defaultMinOS
	}
	v, err := macho.ParseVersion(minOS)
	if err != nil {
		return nil, fmt.Errorf("%w: min os %q: %w", ErrLink, minOS, err)
	}
	target.MinOS = v

	l, err := macholink.New(target)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLink, err)
	}
	defer l.Close()

	entry := opts.Entry
	if entry == "" {
		entry = machOEntry
	}
	// Sign Mach-O ad hoc; the linker derives the identifier from the entry symbol.
	l.SetEntry(entry)

	// Link libSystem stub for hosted macOS execution.
	if !opts.Freestanding {
		sdk := opts.SDK
		if sdk == "" {
			sdk, _ = SDK()
		}
		if sdk == "" {
			return nil, fmt.Errorf("%w: no macOS SDK found: set SDKROOT, "+
				"or install the command line tools (xcode-select --install)", ErrLink)
		}
		l.SetSDK(sdk)
		stub := filepath.Join(sdk, "usr/lib/libSystem.tbd")
		data, err := os.ReadFile(stub)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrLink, stub, err)
		}
		if err := l.AddStub("libSystem", data); err != nil {
			return nil, fmt.Errorf("%w: libSystem: %w", ErrLink, err)
		}
		if opts.Swift {
			swiftStub := filepath.Join(sdk, "usr/lib/swift/libswiftCore.tbd")
			data, err := os.ReadFile(swiftStub)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %w", ErrLink, swiftStub, err)
			}
			if err := l.AddStub("libswiftCore", data); err != nil {
				return nil, fmt.Errorf("%w: libswiftCore: %w", ErrLink, err)
			}
		}

	}

	// Append Vertex runtime unless freestanding or excluded.
	inputs := append([]Input(nil), objs...)
	if !opts.Freestanding && !opts.NoRuntime {
		runtime := Runtime
		if opts.Swift {
			runtime = RuntimeForSwift
		}
		rt, err := runtime(opts.Target)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, rt)
	}
	if opts.Swift {
		bridge, err := SwiftBridge(opts.Target)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, bridge)
	}

	frameworks, err := opts.frameworks(opts.Target)
	if err != nil {
		return nil, err
	}
	named, err := opts.libraries(opts.Target, opts.libraryDirs(opts.Target))
	if err != nil {
		return nil, err
	}

	for _, in := range append(append(append(inputs, opts.Libs...), frameworks...), named...) {
		if err := l.AddFile(in.Name, in.Data); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrLink, in.Name, err)
		}
	}

	img, err := l.Link()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLink, err)
	}
	b, err := img.Bytes()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLink, err)
	}
	return b, nil
}

func amd64PEExe(objs []Input, opts LinkOptions) ([]byte, error) {
	target := pe.Target{
		Machine: pe.MachineAMD64,
		SubArch: pe.MachineAMD64.SubArch(),
		ABI:     pe.ABIMSVC,
		OS:      pe.OSWindows,
	}
	l, err := pelink.New(target)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLink, err)
	}

	entry := opts.Entry
	if entry == "" && opts.Freestanding {
		entry = vsc.EntrySymbol(opts.Target)
	}
	if entry != "" {
		l.SetEntry(entry)
	}

	dirs := opts.libraryDirs(opts.Target)
	l.SetLibPath(dirs...)

	libs, err := opts.libraries(opts.Target, dirs)
	if err != nil {
		return nil, err
	}

	inputs := append([]Input(nil), objs...)
	if !opts.Freestanding && !opts.NoRuntime {
		rt, err := Runtime(opts.Target)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, rt)
	}

	for _, in := range inputs {
		if err := l.AddObject(in.Name, in.Data); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrLink, in.Name, err)
		}
	}
	for _, in := range append(append([]Input(nil), opts.Libs...), libs...) {
		if err := l.AddArchive(in.Name, in.Data); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrLink, in.Name, err)
		}
	}

	img, err := l.Link()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLink, err)
	}
	b, err := img.Bytes()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLink, err)
	}
	return b, nil
}

// libraryDirs returns the search paths for named libraries.
func (opts LinkOptions) libraryDirs(t ir.Target) []string {
	dirs := append([]string(nil), opts.LibDirs...)
	return append(dirs, LibraryDirs(t, opts.Freestanding)...)
}

// libraries resolves named libraries and default runtime libraries in link order.
func (opts LinkOptions) libraries(t ir.Target, dirs []string) ([]Input, error) {
	names := append([]string(nil), opts.LibNames...)
	named := make(map[string]bool, len(names))
	for _, n := range names {
		named[n] = true
	}
	for _, n := range DefaultLibraries(t, opts.Freestanding) {
		if !named[n] {
			names = append(names, n)
		}
	}

	out := make([]Input, 0, len(names))
	for _, name := range names {
		lib, err := findLibrary(t, name, dirs)
		if err != nil {
			return nil, err
		}
		out = append(out, lib)
	}
	return out, nil
}

// frameworks resolves every framework name for one link, in order.
func (opts LinkOptions) frameworks(t ir.Target) ([]Input, error) {
	if len(opts.Frameworks) == 0 {
		return nil, nil
	}
	dirs := append([]string(nil), opts.FrameworkDirs...)
	dirs = append(dirs, sysroot.FrameworkDirs(nil, targetName(t), !opts.Freestanding)...)
	out := make([]Input, 0, len(opts.Frameworks))
	for _, name := range opts.Frameworks {
		fw, err := findFramework(name, dirs)
		if err != nil {
			return nil, err
		}
		out = append(out, fw)
	}
	return out, nil
}

// findFramework looks for name.framework in each directory, and in it the
// text stub the SDK ships or the binary a local build has.
func findFramework(name string, dirs []string) (Input, error) {
	for _, dir := range dirs {
		for _, base := range []string{name + ".tbd", name} {
			path := filepath.Join(dir, name+".framework", base)
			data, err := os.ReadFile(path)
			switch {
			case err == nil:
				return Input{Name: path, Data: data}, nil
			case os.IsNotExist(err):
				continue
			default:
				return Input{}, fmt.Errorf("%w: %s: %w", ErrLink, path, err)
			}
		}
	}
	where := strings.Join(dirs, ", ")
	if where == "" {
		where = "no framework directories"
	}
	return Input{}, fmt.Errorf("%w: cannot find framework %q: no %s.framework in %s",
		ErrLink, name, name, where)
}

// findLibrary searches directory paths for a named library file.
func findLibrary(t ir.Target, name string, dirs []string) (Input, error) {
	names := sysroot.LibraryNames(targetName(t), name)
	for _, dir := range dirs {
		for _, base := range names {
			path := filepath.Join(dir, base)
			data, err := os.ReadFile(path)
			switch {
			case err == nil:
				return Input{Name: path, Data: data}, nil
			case os.IsNotExist(err):
				continue
			default:
				return Input{}, fmt.Errorf("%w: %s: %w", ErrLink, path, err)
			}
		}
	}
	where := strings.Join(dirs, ", ")
	if where == "" {
		where = "no library directories (name one with -L)"
	}
	return Input{}, fmt.Errorf("%w: cannot find library %q: no %s in %s",
		ErrLink, name, strings.Join(names, " or "), where)
}

// machOEntry is the default entry point symbol for Mach-O executables.
const machOEntry = "_main"

// defaultMinOS is the fallback deployment target when none is specified.
const defaultMinOS = "11.0"
