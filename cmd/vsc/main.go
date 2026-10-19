// Command vsc is the command-line driver for the Vertex compiler.
//
// The command delegates CLI argument parsing and driver execution to internal/cli.
package main

import (
	"os"
	"runtime/debug"
	"runtime/pprof"

	"github.com/vertex-language/vsc/internal/cli"
)

func main() { os.Exit(run()) }

// run executes the compiler CLI, enabling optional CPU profiling via VSC_CPUPROFILE.
func run() int {
	// Default GOGC=200 reduces GC overhead for short-lived compiler workloads.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(200)
	}
	if path := os.Getenv("VSC_CPUPROFILE"); path != "" {
		if f, err := os.Create(path); err == nil {
			defer f.Close()
			if pprof.StartCPUProfile(f) == nil {
				defer pprof.StopCPUProfile()
			}
		}
	}
	// Allow spawning worker processes for imported package compilation.
	cli.Workers = true
	return cli.Run(os.Args[1:], os.Stdout, os.Stderr)
}
