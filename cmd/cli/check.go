package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/vertex-language/vsc"
)

// cmdCheck runs the front end and prints what it found.
//
// It stops after the checker: a program that typechecks may still be
// one this compiler cannot lower, and saying so here would report a
// missing feature as though it were a mistake in the source.
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
	srcs, err := sources(fs.Args())
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	_, diags := vsc.Compile(srcs, c.options(target, vsc.Checked))
	if printDiags(stderr, diags) {
		return exitDiags
	}
	return exitOK
}
