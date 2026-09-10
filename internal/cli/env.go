package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/build/sysroot"
)

// cmdEnv prints what the compiler resolved before compiling
// anything.
//
// Everything here is something a build depends on and cannot see:
// which target a bare invocation picks, what symbol a program starts
// at, and where the platform's libraries were found. A link that
// fails for want of an SDK is a great deal easier to understand when
// `vsc env` has already said there is none.
func cmdEnv(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	target, err := c.resolve()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}

	host := vsc.HostName()
	if host == "" {
		host = "(not a target vsc models)"
	}
	fmt.Fprintf(stdout, "target\t%s\n", c.target)
	fmt.Fprintf(stdout, "host\t%s\n", host)
	fmt.Fprintf(stdout, "use\t%s\n", target.Use())
	fmt.Fprintf(stdout, "module\t%s\n", c.module)
	fmt.Fprintf(stdout, "entry\t%s\n", vsc.EntrySymbol(target))
	fmt.Fprintf(stdout, "prefix\t%q\n", vsc.SymbolPrefix(target))
	fmt.Fprintf(stdout, "targets\t%s\n", strings.Join(vsc.Targets(), ", "))

	// Where a link for this target finds the platform's libraries, and
	// what it links against when the program named none.
	//
	// Both are resolved rather than configured — an SDK from xcrun on
	// one target, an MSVC toolset and a Windows SDK from a walk over
	// two versioned installations on the other — so an empty answer
	// here is the whole explanation for a link that has not been run
	// yet. The SDK line is Mach-O's, since Windows has no single
	// directory to name and libdirs is that answer.
	if sysroot.IsMacOS(c.target) {
		sdk, ok := build.SDK()
		if !ok {
			sdk = "(none found: set SDKROOT, or xcode-select --install)"
		}
		fmt.Fprintf(stdout, "sdk\t%s\n", sdk)
	}
	dirs := build.LibraryDirs(target, false)
	if len(dirs) == 0 {
		fmt.Fprintf(stdout, "libdirs\t(none found)\n")
	}
	for i, dir := range dirs {
		label := "libdirs"
		if i > 0 {
			label = ""
		}
		fmt.Fprintf(stdout, "%s\t%s\n", label, dir)
	}
	if libs := build.DefaultLibraries(target, false); len(libs) > 0 {
		fmt.Fprintf(stdout, "libs\t%s\n", strings.Join(libs, ", "))
	}

	if _, ok := build.Host(); !ok {
		fmt.Fprintln(stdout, "note\tthis machine has no backend; build can emit vil and vir only")
	}
	return exitOK
}
