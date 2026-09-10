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
	"github.com/vertex-language/vsc/runtime"
)

// ErrLink is a link this package could not complete. The error it
// wraps says which symbol, which file, or which target.
var ErrLink = errors.New("build: link")

// An Input is one file the linker reads: an object this compiler
// wrote, or an archive or stub from the platform. Bytes rather than a
// path, because an object never has to reach the filesystem to be
// linked and a caller that generated one should not have to put it
// somewhere first.
type Input struct {
	Name string
	Data []byte
}

// LinkOptions are what the caller decides about the executable.
type LinkOptions struct {
	// Target is the machine the objects were compiled for. The
	// executable is refused rather than mislinked where this package
	// has no linker for it.
	Target ir.Target

	// Entry is the symbol the image starts at. Empty means the
	// platform's, which is not the same symbol on both of them.
	//
	// On Mach-O it is the program's own main: dyld jumps straight to
	// it, so the image entry and what vsc.EntrySymbol names are one
	// symbol. On PE they are two — the image starts in the CRT, at a
	// mainCRTStartup that initialises the runtime and calls main
	// afterwards — and naming main here would start the program with
	// no runtime under it. So a PE link leaves this empty and lets
	// the linker infer the startup from which main the program
	// defined, which is what link.exe does and what the CRT expects.
	//
	// Setting it is how a caller overrides both.
	Entry string

	// MinOS is the deployment target recorded in the image. Empty
	// takes the same default an object gets.
	MinOS string

	// SDK is where the platform's libraries are found. Empty means
	// ask the host, which is what SDK() does. It is Mach-O's: a
	// Windows link finds two installations rather than one and has no
	// single directory to be told about, and LibDirs is what names
	// them there.
	SDK string

	// LibDirs are directories to look for a named library in, before
	// the platform's own. It is -L, and it is what a program links
	// against something the platform did not install.
	LibDirs []string

	// LibNames are libraries to resolve by name against LibDirs and
	// the platform's directories, in order, and link after the
	// objects. The platform's own runtime is added after these
	// unless the link is freestanding — a program that names one
	// library wants that library as well as a runtime, not instead of
	// one.
	LibNames []string

	// Freestanding links no platform libraries at all: no libSystem,
	// no SDK. A program that says this undertakes to define
	// everything it names, and gets an undefined-symbol error rather
	// than a silent resolution if it does not.
	Freestanding bool

	// Libs are archives and stubs to link after the objects. Order is
	// preserved, because a static link is order-sensitive and
	// reordering it would be this package deciding something the
	// caller said.
	Libs []Input
}

// Executable links objects into a runnable image and returns its
// bytes.
//
// This is the step past Object, and the last one that is entirely the
// compiler's own: what comes out needs no toolchain to have been
// installed and no `ld` to have been run. The linker is
// vertex-language's, shared with vcc, and takes bytes and returns
// bytes — which is why nothing here writes a temporary file.
func Executable(objs []Input, opts LinkOptions) ([]byte, error) {
	if len(objs) == 0 {
		return nil, fmt.Errorf("%w: nothing to link", ErrLink)
	}
	switch opts.Target.Use() {
	case "aarch64/macos":
		return aarch64MachOExe(objs, opts)
	case "x86_64/windows":
		return amd64PEExe(objs, opts)
	}
	return nil, fmt.Errorf("%w: %s", ErrTarget, opts.Target.Use())
}

func aarch64MachOExe(objs []Input, opts LinkOptions) ([]byte, error) {
	target := macho.Target{
		// The subtype is named rather than left zero: Mach-O's backend
		// registry keys on both halves and matches the subtype
		// exactly. arm64's "any implementation" happens to be 0 and
		// x86_64's does not, so leaving it out is a bug that only the
		// second target finds.
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
	// The image is signed ad hoc, since an arm64 macOS binary will not
	// execute otherwise, and the linker takes its signing identifier
	// from the entry symbol when nothing else names it. That makes
	// every program built here identify as `_main`, where `ld` would
	// use the output's base name. It is cosmetic — the signature is
	// valid and the program runs — and it is not fixable from this
	// side: macho/link takes an install name only for a dylib and has
	// no other way to say what the image is called.
	l.SetEntry(entry)

	// libSystem is what a hosted program needs and the only thing it
	// needs: dyld calls the entry point directly, so there is no crt
	// to find, and the stub beside the SDK's headers is what resolves
	// malloc and the rest when something starts calling them.
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

		// The Swift runtime, which is part of the system on macOS:
		// libswiftCore lives in /usr/lib/swift and is in the shared
		// cache, so a program that uses it needs no rpath and nothing
		// copied beside it.
		//
		// Added whether or not the program calls into it. A stub
		// binds only the symbols something actually references, so a
		// program that uses none of it pays a load command; a program
		// that says `print` would otherwise not link at all, and
		// which of the two it is is not something this layer can see.
		// A machine whose SDK has no Swift is left alone rather than
		// refused: the program may not need it.
		swiftStub := filepath.Join(sdk, "usr/lib/swift/libswiftCore.tbd")
		if data, err := os.ReadFile(swiftStub); err == nil {
			if err := l.AddStub("libswiftCore", data); err != nil {
				return nil, fmt.Errorf("%w: libswiftCore: %w", ErrLink, err)
			}
		}
	}

	// The runtime after the program and before the platform, which is
	// where it sits: it is what the program calls, and it calls
	// libSystem in turn. A freestanding link takes neither.
	inputs := append([]Input(nil), objs...)
	if !opts.Freestanding {
		rt, err := Runtime(opts.Target)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, rt)
	}

	// Libraries the caller named, resolved against the search path
	// the same way the other target's are. DefaultLibraries has no
	// answer here — libSystem is added above, by path — so this is
	// empty for a link that named none, which is most of them.
	named, err := opts.libraries(opts.Target, opts.libraryDirs(opts.Target))
	if err != nil {
		return nil, err
	}

	// The objects first and the libraries after, which is the order a
	// static link resolves in.
	for _, in := range append(append(inputs, opts.Libs...), named...) {
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

	// Empty is the CRT's, which the linker works out for itself from
	// the main the program defined — see LinkOptions.Entry. The one
	// case with no CRT to defer to is a freestanding link: there is
	// no mainCRTStartup to find, so the image starts where the
	// program does.
	entry := opts.Entry
	if entry == "" && opts.Freestanding {
		entry = vsc.EntrySymbol(opts.Target)
	}
	if entry != "" {
		l.SetEntry(entry)
	}

	// The search path is handed over as well as used here, because
	// the linker resolves names of its own: a /DEFAULTLIB directive
	// inside a CRT object names a library nobody wrote down, and it
	// is the linker that has to find it.
	dirs := opts.libraryDirs(opts.Target)
	l.SetLibPath(dirs...)

	libs, err := opts.libraries(opts.Target, dirs)
	if err != nil {
		return nil, err
	}

	// The runtime after the program and before the platform, which is
	// where it sits: it is what the program calls, and it calls the
	// CRT in turn. A freestanding link takes neither.
	inputs := append([]Input(nil), objs...)
	if !opts.Freestanding {
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
	// The archives after every object, which is where a static link
	// resolves them: an archive contributes only what something
	// already in the link needs, so one placed before its callers
	// contributes nothing.
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

// -l and -L: how a library name becomes bytes.
//
// The vertex-language linkers take bytes, so resolving a name is this
// package's job — which is the right place for it anyway, because the
// spelling a name resolves through is a fact of the target's platform
// and this package is what knows the target.

// libraryDirs is where a named library is looked for: the caller's
// first, then the platform's own.
func (opts LinkOptions) libraryDirs(t ir.Target) []string {
	dirs := append([]string(nil), opts.LibDirs...)
	return append(dirs, LibraryDirs(t, opts.Freestanding)...)
}

// libraries resolves every named library for one link, in order: the
// ones the caller named, then the platform's default runtime for the
// ones it did not.
//
// The runtime comes last and comes always, because a program that
// says nothing about libraries still has to reach main and one that
// does say something still has to. Last is also where a static link
// wants it: an archive satisfies the references to its left, so the
// CRT after the libraries that call into it resolves and the reverse
// does not.
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

// findLibrary resolves one library name against the search list and
// reads it.
//
// A name that resolves nowhere is an error naming what was looked for
// and where, because "cannot find ucrt" without the search list is a
// message that sends the reader to a debugger.
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
				// Found and unreadable is not "keep looking": a
				// directory the caller named holds a library it
				// cannot open, and searching past it would report the
				// wrong problem.
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

// Runtime is the runtime, compiled for a target: the object holding
// vertex_alloc, vertex_retain and vertex_release.
//
// It is built rather than found. runtime/ emits it as a VIR module
// and it goes through the same instruction selection and the same
// object writer as the program that calls it — so there is no
// prebuilt library to ship, nothing to compile ahead of time, and no
// second toolchain. A target that can compile a program can build the
// runtime for it by construction.
//
// Linking it is Executable's business and it does so unless the link
// is freestanding. This is exported for a caller doing its own
// linking, which needs the same object and should not have to know
// how to make it.
func Runtime(target ir.Target) (Input, error) {
	m, err := runtime.Module(target, runtime.Options{SymbolPrefix: vsc.SymbolPrefix(target)})
	if err != nil {
		return Input{}, fmt.Errorf("%w: runtime: %w", ErrLink, err)
	}
	obj, err := Object(m, Options{})
	if err != nil {
		return Input{}, fmt.Errorf("%w: runtime: %w", ErrLink, err)
	}
	return Input{Name: "vertex_runtime.o", Data: obj}, nil
}

// machOEntry is the symbol a Mach-O executable starts at: `main` with
// the platform's underscore, which is vsc.EntrySymbol's answer for
// every target this package links.
const machOEntry = "_main"

// defaultMinOS is the deployment target an image records when the
// caller named none. It is the same one Object writes, so an object
// and the executable it goes into never disagree about the platform
// they expect.
const defaultMinOS = "11.0"
