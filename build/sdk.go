package build

import (
	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build/sysroot"
)

// Where the platform's own libraries are found.
//
// This was one function for one platform. With a second hosted target
// it became a package, which is what its own documentation said would
// happen: the macOS answer is one path from xcrun, the Windows answer
// is a walk over two versioned installations, and the two have
// nothing in common but the question. See build/sysroot.
//
// What is left here is the question asked in this package's own
// vocabulary — an ir.Target rather than a target name — so that a
// caller holding a target does not have to spell it back into a
// string to find out where its libraries are.

// SDK is the macOS SDK this host links against, and whether one was
// found.
//
// It is exported and it is macOS-shaped because that is what a caller
// wants it for: a Mach-O link with no SDK is the one failure worth
// reporting before the build rather than after it. The Windows
// equivalent is not a single directory and so is not a single answer;
// LibraryDirs is that one.
func SDK() (string, bool) { return sysroot.SDK(nil) }

// LibraryDirs is where a link for t looks for the platform's
// libraries, in order, and is empty for a freestanding one.
//
// `vsc env` prints it, which is the point: a Windows link that fails
// for want of a toolset is a great deal easier to read when the
// search list is already on screen.
func LibraryDirs(t ir.Target, freestanding bool) []string {
	return sysroot.LibraryDirs(nil, targetName(t), !freestanding)
}

// DefaultLibraries is the platform runtime a hosted link for t is
// given when the program named nothing.
func DefaultLibraries(t ir.Target, freestanding bool) []string {
	return sysroot.DefaultLibraries(targetName(t), !freestanding)
}

// targetName is the target-table name for an ir.Target: what sysroot
// dispatches on.
//
// vsc.SymbolPrefix answers a related question from the same table, and
// this one is asked the same way — by use path, which is the only
// thing an ir.Target carries and the only thing it needs to.
func targetName(t ir.Target) string {
	for _, row := range vsc.Targets() {
		if rt, ok := vsc.LookupTarget(row); ok && rt.Use() == t.Use() {
			return row
		}
	}
	return ""
}
