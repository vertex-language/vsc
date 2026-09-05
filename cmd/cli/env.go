package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
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

	sdk, ok := build.SDK()
	if !ok {
		sdk = "(none found: set SDKROOT, or xcode-select --install)"
	}
	fmt.Fprintf(stdout, "sdk\t%s\n", sdk)

	if _, ok := build.Host(); !ok {
		fmt.Fprintln(stdout, "note\tthis machine has no backend; build can emit vil and vir only")
	}
	return exitOK
}
