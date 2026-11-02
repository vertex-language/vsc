package cli

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/vertex-language/vsc"
)

// cmdCheck runs the compiler as far as a program's source can be wrong, past
// type checking through the mandatory SIL passes, and prints diagnostics. The
// packages the program imports from its own module are read the same way, as
// a build compiles them; another module's packages have been checked by theirs.
func cmdCheck(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
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
	srcs, err := sources(fs.Args(), target)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	c.notice = func(msg string) { fmt.Fprintln(stderr, "vsc:", msg) }
	u, diags := vsc.Compile(srcs, c.options(target, vsc.Canonical))
	failed := printDiags(stderr, diags)
	for _, p := range u.Packages {
		if c.main == nil {
			break
		}
		if _, ok := under(filepath.ToSlash(p.Dir), filepath.ToSlash(c.main.root.Dir)); !ok {
			continue
		}
		opts := c.options(target, vsc.Canonical)
		opts.Module = p.Name
		_, diags := vsc.Compile(p.Sources, opts)
		failed = printDiags(stderr, diags) || failed
	}
	if failed {
		return exitDiags
	}
	return exitOK
}
