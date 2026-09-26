// Package cli implements the vsc command line.
package cli

import (
	"fmt"
	"io"
	"os"
)

// Exit codes.
const (
	exitOK    = 0 // no error diagnostics
	exitDiags = 1 // the input had errors
	exitUsage = 2 // the invocation or I/O had errors
)

const usage = `vsc — the Vertex compiler

Usage:

    vsc build  [flags] [files...]  compile and link; with --emit, stop earlier
    vsc run    [flags] [files...]  build to a temporary path and run it
    vsc check  [flags] [files...]  parse and typecheck; print diagnostics
    vsc ast    [flags] [file]      parse and dump the syntax tree
    vsc tokens [flags] [file]      dump the token stream
    vsc env    [flags]             print the resolved target and SDK

A file of "-" (or no file) reads standard input.

Common flags:
    -target T       target to build for (default: this host)
    -module name    the module being compiled (default: main)
    -P dir          a package directory root, for string-form imports (repeatable)
    -replace p=dir  use a local checkout for the imported package p (repeatable)
    -offline        never fetch a package: use the cache as it is
    -update         fetch imported packages again, even if the cache holds them

Flags for build and run:
    --emit exe      compile and link (the default)
    --emit obj      compile to an object file
    --emit lib      compile and link a shared library (aarch64-android)
    --emit vir      stop after lowering and print the machine IR
    --emit sil      stop after the ownership passes and print the IR, as SIL
    --emit interface  write the module's public face, for another module
                      to be compiled against
    -I dir          look for imported modules here (repeatable)
    -o file         write output here ("-" is standard output, for vir and sil)
    -entry sym      the program's entry symbol (default: the platform's)
    -freestanding   link no platform libraries
    --package-path d  build the package at d (default: this directory, when it has
                      a Package.swift); "vsc run [product]" runs one of its programs

Imported packages:

    import "net/tcp"
    import "ui/window"

A string import names a package rather than a file. A path that is a
folder -- "./util", or one under a -P root -- is compiled from there. A
reserved name, which the language defines, and a path that names a
repository ("github.com/you/thing") are fetched into the package cache
the first time they are wanted and read from it afterwards, so only the
first build of a program touches the network. Cloning is built in: a
fetch needs no git installed.

The cache is VERTEXCACHE, or a "vertex" directory in this machine's own
cache. Everything in it can be deleted; a build fetches what it needs
again. Use -replace to point a package at a checkout you are editing.

The linker is vsc's own, so a build needs no cc, as or ld installed.
Running what it produced is another matter: vsc run needs this
machine's target.

The module name decides the entry point: main in module main is the
program's, and every other module's main is an ordinary function.
That is why -module defaults to main and why building a library means
saying so.

A module is imported by name: "import Geometry" looks for
Geometry.vinterface in each -I directory, in order. An interface
is source -- valid Vertex with the bodies taken out -- which is why
compiling against one needs no separate module format.

Targets: aarch64-macos, aarch64-android and x86_64-windows; vsc env
lists them. Not yet: --emit asm, --emit device, -L, -l, -static.

Exit codes: 0 no errors, 1 diagnostics with errors, 2 usage or I/O.
`

// Run executes one vsc invocation and returns its exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "build":
		return cmdBuild(rest, stdout, stderr)
	case "run":
		return cmdRun(rest, stdout, stderr)
	case "check":
		return cmdCheck(rest, stderr)
	case "ast":
		return cmdAST(rest, stdout, stderr)
	case "tokens":
		return cmdTokens(rest, stdout, stderr)
	case "env":
		return cmdEnv(rest, stdout, stderr)
	case "help", "-h", "--help", "-help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "vsc: unknown command %q\n\n%s", verb, usage)
		return exitUsage
	}
}

// writeOut sends one artifact to a file or standard output ("-" or "").
func writeOut(name string, stdout io.Writer, data []byte, exec bool) error {
	if isStdout(name) {
		_, err := stdout.Write(data)
		return err
	}
	mode := os.FileMode(0o666)
	if exec {
		mode = 0o777
	}
	return os.WriteFile(name, data, mode)
}

func isStdout(name string) bool { return name == "" || name == "-" }

func maxCode(a, b int) int {
	if a > b {
		return a
	}
	return b
}
