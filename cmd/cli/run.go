package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/vertex-language/vsc"
)

// cmdRun builds a program to a temporary path and starts it.
//
// The exit code is the program's, forwarded, because a runner that
// swallows it is a runner that cannot be used in a script. Everything
// after `--` is the program's arguments rather than vsc's, which is
// the convention every tool that runs something else uses.
func cmdRun(args []string, stdout, stderr io.Writer) int {
	args, progArgs := splitArgs(args)

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var bf buildFlags
	bf.register(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if bf.emit != "exe" {
		fmt.Fprintf(stderr, "vsc: run builds a program; --emit %s has nothing to run\n", bf.emit)
		return exitUsage
	}
	// Running what was built means running it here, so a target that
	// is not this machine is a mistake worth catching before the
	// build rather than after it.
	if host := vsc.HostName(); bf.target != host {
		fmt.Fprintf(stderr, "vsc: run needs this machine's target: %q was asked for, this is %q\n",
			bf.target, host)
		return exitUsage
	}

	dir, err := os.MkdirTemp("", "vsc-run-")
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	defer os.RemoveAll(dir)

	bf.output = filepath.Join(dir, "prog")
	if _, code := doBuild(&bf, fs.Args(), stdout, stderr); code != exitOK {
		return code
	}

	cmd := exec.Command(bf.output, progArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	return exitOK
}

// splitArgs divides vsc's arguments from the program's at the first
// bare `--`.
func splitArgs(args []string) (mine, theirs []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}
