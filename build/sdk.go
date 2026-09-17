package build

import (
	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build/sysroot"
)

// SDK returns the path to the macOS SDK for Mach-O linking, and whether one was found.
func SDK() (string, bool) { return sysroot.SDK(nil) }

// LibraryDirs returns the ordered search paths for platform libraries for target t.
func LibraryDirs(t ir.Target, freestanding bool) []string {
	return sysroot.LibraryDirs(nil, targetName(t), !freestanding)
}

// DefaultLibraries returns the default platform runtime libraries for target t.
func DefaultLibraries(t ir.Target, freestanding bool) []string {
	return sysroot.DefaultLibraries(targetName(t), !freestanding)
}

// targetName returns the target name corresponding to an ir.Target.
func targetName(t ir.Target) string {
	for _, row := range vsc.Targets() {
		if rt, ok := vsc.LookupTarget(row); ok && rt.Use() == t.Use() {
			return row
		}
	}
	return ""
}
