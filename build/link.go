package build

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/macho"
	macholink "github.com/vertex-language/macho/link"

	"github.com/vertex-language/vsc"
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

	// Entry is the symbol the program starts at. Empty means the
	// platform's, which is what vsc.EntrySymbol says and what
	// vil/gen gave the program's main.
	Entry string

	// MinOS is the deployment target recorded in the image. Empty
	// takes the same default an object gets.
	MinOS string

	// SDK is where the platform's libraries are found. Empty means
	// ask the host, which is what SDK() does.
	SDK string

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

	// The objects first and the libraries after, which is the order a
	// static link resolves in.
	for _, in := range append(inputs, opts.Libs...) {
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
