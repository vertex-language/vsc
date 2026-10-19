package cli

import (
	"errors"
	"flag"
	"fmt"
	"github.com/vertex-language/vsc/timing"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/vertex-language/vsc"
)

// cmdRun builds a program to a temporary path and executes it.
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
	// Ensure the build target matches the host machine for execution.
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

	// Format binary name with platform executable suffix (e.g. .exe on Windows).
	target, err := bf.resolve()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	bf.output = vsc.ImageName(target, filepath.Join(dir, "prog"))
	_, code := doBuild(&bf, fs.Args(), stdout, stderr)
	timing.Print(stderr)
	if code != exitOK {
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

// splitArgs separates vsc flags from program arguments at the first "--".
func splitArgs(args []string) (mine, theirs []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}
