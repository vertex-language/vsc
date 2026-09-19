// Command vsc is the Vertex compiler.
//
// It has nothing of its own: everything it does is cli.Run, which is
// a wrapper over the vsc package, which is the phases composed. Three
// lines here rather than none because a command is a module's entry
// point and a library is not, and keeping them apart is what lets a
// program embed the compiler without embedding the command.
package main

import (
	"os"
	"runtime/pprof"

	"github.com/vertex-language/vsc/internal/cli"
)

func main() { os.Exit(run()) }

// run is cli.Run, under a CPU profile when VSC_CPUPROFILE names a file
// to write one to -- for finding where a slow build's time goes.
func run() int {
	if path := os.Getenv("VSC_CPUPROFILE"); path != "" {
		if f, err := os.Create(path); err == nil {
			defer f.Close()
			if pprof.StartCPUProfile(f) == nil {
				defer pprof.StopCPUProfile()
			}
		}
	}
	return cli.Run(os.Args[1:], os.Stdout, os.Stderr)
}
