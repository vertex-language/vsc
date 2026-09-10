package build

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"

	"github.com/vertex-language/ir"
	amd64lower "github.com/vertex-language/ir/lower/amd64"
	arm64lower "github.com/vertex-language/ir/lower/arm64"

	amd64pe "github.com/vertex-language/amd64/obj/pe"
	arm64macho "github.com/vertex-language/arm64/obj/macho"
	machocore "github.com/vertex-language/macho"
)

// ErrTarget is a target this package has no backend or no object
// writer for yet.
var ErrTarget = errors.New("build: unsupported target")

// Options are what the caller decides about the object file.
type Options struct {
	// Platform and MinOS are what a Mach-O object records about where
	// it expects to run. They are ignored for other formats.
	Platform machocore.Platform
	MinOS    string
}

// Object lowers a VIR module and returns the bytes of an object file
// for its target.
func Object(m *ir.Module, opts Options) ([]byte, error) {
	switch m.Use() {
	case "aarch64/macos":
		return aarch64MachO(m, opts)
	case "x86_64/windows":
		return amd64PE(m, opts)
	}
	return nil, fmt.Errorf("%w: %s", ErrTarget, m.Use())
}

func aarch64MachO(m *ir.Module, opts Options) ([]byte, error) {
	// Mach-O prefixes a symbol with an underscore, and the library
	// calls the backend invents -- a soft-float helper, a memcpy --
	// need it as much as anything else does. The module's own symbols
	// arrive with it already applied: the prefix belongs to the
	// language rather than to the IR, so lower puts it on and nothing
	// below here renames anything. Apple's variadic convention is the
	// one this platform uses.
	o, err := arm64lower.Lower(m, arm64lower.Options{
		LibcallPrefix: "_",
		Variadic:      arm64lower.VariadicDarwin,
	})
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	if opts.Platform == 0 {
		opts.Platform = machocore.PlatformMacOS
	}
	if opts.MinOS == "" {
		opts.MinOS = defaultMinOS
	}
	var buf bytes.Buffer
	if err := arm64macho.Write(&buf, o, arm64macho.Options{
		Platform: opts.Platform,
		MinOS:    opts.MinOS,
	}); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	return buf.Bytes(), nil
}

// amd64PE lowers for x86-64 and writes a COFF object.
//
// Nothing is prefixed: COFF spells a C symbol the way the source did,
// which is why the module arrives with its names already final and
// why the libcalls the backend invents -- a memcpy, a soft-float
// helper -- are named plainly. The Microsoft argument convention is
// not asked for either; it is in the module's layout, which says
// abi "ms", and the backend reads it there.
func amd64PE(m *ir.Module, opts Options) ([]byte, error) {
	o, err := amd64lower.Lower(m, amd64lower.Options{})
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	var buf bytes.Buffer
	// The .file symbol a debugger and a map file want. Platform and
	// MinOS are Mach-O's and say nothing here, which is what their
	// documentation on Options already promises.
	if err := amd64pe.Write(&buf, o, amd64pe.Options{File: m.Name()}); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	return buf.Bytes(), nil
}

// Host is the target this machine runs, or the zero Target where this
// package has no backend for it.
//
// A compiler that cannot be asked "the machine I am on" makes every
// caller write the same switch.
func Host() (ir.Target, bool) {
	switch {
	case runtime.GOARCH == "arm64" && runtime.GOOS == "darwin":
		return ir.AArch64MacOS, true
	case runtime.GOARCH == "amd64" && runtime.GOOS == "windows":
		return ir.X86_64Windows, true
	}
	return ir.Target{}, false
}
