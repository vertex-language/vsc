// Package cli implements the vsc command line.
//
// It is a wrapper. Everything about compiling Vertex — the phases,
// the targets, the symbols, the link — belongs to the vsc package and
// to build, and this package is what a command adds to them: flags,
// where an artifact lands, standard input, the caret under a
// diagnostic, and an exit code.
//
// The rule is that nothing here decides anything a library caller
// would also have to decide. Two copies of the pipeline is two places
// for the phases to drift apart, which is the failure the phase model
// exists to prevent — so when a verb needs something the library does
// not expose, the fix is to expose it rather than to reach around.
// The target table moved into vsc for exactly that reason.
//
// Run is the entire API. Everything else is unexported: the CLI is a
// consumer of the library, never a library itself.
package cli

import (
	"fmt"
	"io"
	"os"
)

// Exit codes, shaped so `vsc check f.vs && echo ok` means what it
// looks like.
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

Flags for build and run:
    --emit exe      compile and link (the default)
    --emit obj      compile to an object file
    --emit vir      stop after lowering and print the machine IR
    --emit vil      stop after the ownership passes and print the IR, as SIL
    -o file         write output here ("-" is standard output, for vir and vil)
    -entry sym      the program's entry symbol (default: the platform's)
    -freestanding   link no platform libraries

The linker is vsc's own, so a build needs no cc, as or ld installed.
Running what it produced is another matter: vsc run needs this
machine's target.

The module name decides the entry point: main in module main is the
program's, and every other module's main is an ordinary function.
That is why -module defaults to main and why building a library means
saying so.

Not yet: --emit asm, --emit device, -I, -L, -l, -static, cross-target
builds. The target table has one row.

Exit codes: 0 no errors, 1 diagnostics with errors, 2 usage or I/O.
`

// Run executes one vsc invocation and returns its exit code. It is
// the package's entire surface: cmd/vsc calls it and nothing else.
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

// writeOut sends one artifact to a file or to standard output. "-"
// and "" mean standard output, which is what `-o -` asks for.
//
// An executable is written 0o777 before umask and everything else
// 0o666, because the one artifact a person runs is the one that has
// to be runnable without a chmod.
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
