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
	arm64elf "github.com/vertex-language/arm64/obj/elf"
	arm64macho "github.com/vertex-language/arm64/obj/macho"
	machocore "github.com/vertex-language/macho"
)

// ErrTarget indicates an unsupported target architecture or platform.
var ErrTarget = errors.New("build: unsupported target")

// Options configures object file emission.
type Options struct {
	// Platform and MinOS specify target platform and minimum OS version for Mach-O.
	Platform machocore.Platform
	MinOS    string
}

// Object lowers a VIR module and returns the bytes of an object file
// for its target.
func Object(m *ir.Module, opts Options) ([]byte, error) {
	// f16 and bf16 before any backend sees them: none of these selects
	// half instructions, and the IR's legalizer carries each in an i32
	// and does its arithmetic in f32 (see ir.Module.LegalizeHalf).
	m.LegalizeHalf()
	if err := m.Err(); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	switch m.Use() {
	case "aarch64/macos":
		return aarch64MachO(m, opts)
	case "x86_64/windows":
		return amd64PE(m, opts)
	case "aarch64/android":
		return aarch64ELF(m)
	}
	return nil, fmt.Errorf("%w: %s", ErrTarget, m.Use())
}

func aarch64MachO(m *ir.Module, opts Options) ([]byte, error) {
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
		Platform:    opts.Platform,
		MinOS:       opts.MinOS,
		Subsections: true,
	}); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	return buf.Bytes(), nil
}

// aarch64ELF lowers for base AAPCS64, as Android has it, and writes an ELF
// relocatable object. Libcalls are unprefixed and variadic arguments go
// where named ones would.
func aarch64ELF(m *ir.Module) ([]byte, error) {
	o, err := arm64lower.Lower(m, arm64lower.Options{
		Variadic: arm64lower.VariadicAAPCS64,
	})
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	var buf bytes.Buffer
	if err := arm64elf.Write(&buf, o); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	return buf.Bytes(), nil
}

// amd64PE lowers for x86-64 and writes a COFF object file.
func amd64PE(m *ir.Module, opts Options) ([]byte, error) {
	o, err := amd64lower.Lower(m, amd64lower.Options{})
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	var buf bytes.Buffer
	if err := amd64pe.Write(&buf, o, amd64pe.Options{File: m.Name()}); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	return buf.Bytes(), nil
}

// Host returns the host target, or a zero Target if unsupported.
func Host() (ir.Target, bool) {
	switch {
	case runtime.GOARCH == "arm64" && runtime.GOOS == "darwin":
		return ir.AArch64MacOS, true
	case runtime.GOARCH == "amd64" && runtime.GOOS == "windows":
		return ir.X86_64Windows, true
	}
	return ir.Target{}, false
}
