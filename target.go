package vsc

import (
	"path/filepath"
	"runtime"
	"sort"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil/gen"
)

// target defines platform-specific configuration: IR target, symbol prefix, and executable suffix.
type target struct {
	name   string
	ir     ir.Target
	prefix string // "_" on Mach-O, empty on ELF and COFF
	suffix string // ".exe" on PE, empty elsewhere
}

// targets lists all fully supported compilation targets.
var targets = []target{
	{name: "aarch64-android", ir: ir.AArch64Android, prefix: ""},
	{name: "aarch64-macos", ir: ir.AArch64MacOS, prefix: "_"},
	{name: "x86_64-windows", ir: ir.X86_64Windows, prefix: "", suffix: ".exe"},
}

// LookupTarget returns the IR target matching the given target name.
func LookupTarget(name string) (ir.Target, bool) {
	for _, t := range targets {
		if t.name == name {
			return t.ir, true
		}
	}
	return ir.Target{}, false
}

// Targets returns all supported target names sorted alphabetically.
func Targets() []string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.name)
	}
	sort.Strings(names)
	return names
}

// HostName returns the target name for the host machine, or "" if unsupported.
func HostName() string {
	switch {
	case runtime.GOARCH == "arm64" && runtime.GOOS == "darwin":
		return "aarch64-macos"
	case runtime.GOARCH == "amd64" && runtime.GOOS == "windows":
		return "x86_64-windows"
	}
	return ""
}

// HostTarget returns the IR target for the host machine.
func HostTarget() (ir.Target, bool) { return LookupTarget(HostName()) }

// SymbolPrefix returns the symbol prefix required by the target object format (e.g. "_" for Mach-O).
func SymbolPrefix(t ir.Target) string {
	for _, row := range targets {
		if row.ir.Use() == t.Use() {
			return row.prefix
		}
	}
	return ""
}

// ImageName appends the target platform's executable suffix to path if it lacks an extension.
func ImageName(t ir.Target, path string) string {
	if filepath.Ext(path) != "" {
		return path
	}
	for _, row := range targets {
		if row.ir.Use() == t.Use() {
			return path + row.suffix
		}
	}
	return path
}

// EntryModule is the default module name containing the program's main entry point.
const EntryModule = gen.EntryModule

// EntrySymbol returns the mangled entry point symbol name for the given target.
func EntrySymbol(t ir.Target) string { return SymbolPrefix(t) + gen.EntryName }
