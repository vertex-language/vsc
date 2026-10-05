package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/vertex-language/vsc"
)

// cmdCheck runs the front end and prints diagnostics.
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
	_, diags := vsc.Compile(srcs, c.options(target, vsc.Checked))
	if printDiags(stderr, diags) {
		return exitDiags
	}
	return exitOK
}
