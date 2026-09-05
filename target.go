package vsc

import (
	"runtime"
	"sort"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/vil/gen"
)

// The targets, and what a target's name decides.
//
// A name decides two things at once — the type model the front end
// sizes against, and the architecture and container below the IR —
// and both halves belong in one table so that a name means one thing.
// The first half is not here yet: every target this compiler models
// is 64-bit little-endian with the same widths, so there is nothing
// for the front end to vary on and nothing to write down. When there
// is, it goes in the row beside the rest.
//
// The vocabulary is the family's, shared with vcc: arch-os, with the
// architecture spelled as the backend spells it.

// A target is one row: the name, what the IR is built for, and what
// the container does to a symbol.
type target struct {
	name   string
	ir     ir.Target
	prefix string // "_" on Mach-O, empty on ELF and COFF
}

// The table. One row per target the whole toolchain can carry a
// program through, which today is one — a row is cheap and a row that
// compiles but cannot be linked is a promise this compiler does not
// keep.
var targets = []target{
	{name: "aarch64-macos", ir: ir.AArch64MacOS, prefix: "_"},
}

// LookupTarget is the target of that name.
func LookupTarget(name string) (ir.Target, bool) {
	for _, t := range targets {
		if t.name == name {
			return t.ir, true
		}
	}
	return ir.Target{}, false
}

// Targets is every target name, sorted, for a message that has to
// list them.
func Targets() []string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.name)
	}
	sort.Strings(names)
	return names
}

// HostName is the target name for the machine this is running on, or
// "" where it is not one this compiler models.
//
// A compiler that cannot be asked "the machine I am on" makes every
// caller write the same switch, and a caller that writes it will get
// it wrong somewhere.
func HostName() string {
	switch {
	case runtime.GOARCH == "arm64" && runtime.GOOS == "darwin":
		return "aarch64-macos"
	}
	return ""
}

// HostTarget is the target this machine runs, and whether it is one.
func HostTarget() (ir.Target, bool) { return LookupTarget(HostName()) }

// SymbolPrefix is what this target writes in front of every symbol.
//
// Mach-O wants an underscore on all of them and ELF and COFF want
// nothing, which is a fact about the container rather than about the
// architecture. `nm` on a `swiftc -c` object says so plainly: a
// function SIL calls @$s2m21fyS2iF is `_$s2m21fyS2iF` in the object,
// and the entry point SIL calls @main is `_main`.
//
// It is exported because a caller that links what this compiler
// produced has to name a symbol in it, and deriving the prefix a
// second time is how a program ends up linking against a name nothing
// defines.
//
// A target with no row answers "", which is the right answer for the
// two containers that want nothing and the only safe one for a target
// this table has never heard of.
func SymbolPrefix(t ir.Target) string {
	for _, row := range targets {
		if row.ir.Use() == t.Use() {
			return row.prefix
		}
	}
	return ""
}

// EntryModule is the module whose main is the program's entry point.
// A caller that has to name a module — a command's -module default,
// a build system's — should say this rather than "main", so that the
// rule lives in one place.
const EntryModule = gen.EntryModule

// EntrySymbol is the object-file symbol a program built for t starts
// at: what a linker is told to make the entry point, and what the
// entry point's definition is called in the object.
//
// Both halves come from where they are decided — vil/gen names the
// entry point, this table says what the platform does to a name — so
// that the two cannot drift into disagreeing.
func EntrySymbol(t ir.Target) string { return SymbolPrefix(t) + gen.EntryName }
