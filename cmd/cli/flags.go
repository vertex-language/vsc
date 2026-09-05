package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc"
)

// The flag sets, and the one place a target is resolved.
//
// Everything here is command-line work: reading a target name,
// reading files, and turning "-" into standard input. What any of it
// means is the library's.

// common is the flag set every verb shares.
type common struct {
	target string
	module string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.target, "target", vsc.HostName(), "target to build for")
	fs.StringVar(&c.module, "module", vsc.EntryModule, "the module being compiled")
}

// resolve turns the target flag into a target.
//
// It is checked here rather than left to the library so the message
// can name the flag that fixes it — the one thing a library cannot
// say, and the one thing a person at a terminal wants to read.
func (c *common) resolve() (ir.Target, error) {
	if c.target == "" {
		return ir.Target{}, fmt.Errorf("this host is not a target vsc models; name one with -target (known: %s)",
			strings.Join(vsc.Targets(), ", "))
	}
	t, ok := vsc.LookupTarget(c.target)
	if !ok {
		return ir.Target{}, fmt.Errorf("unknown target %q (known: %s)",
			c.target, strings.Join(vsc.Targets(), ", "))
	}
	return t, nil
}

// options is what the library is asked for, stopping where the verb
// wants it stopped.
func (c *common) options(t ir.Target, stop vsc.Phase) vsc.Options {
	return vsc.Options{Module: c.module, Target: t, Stop: stop}
}

// source reads one input. "" and "-" mean standard input, which the
// library has no way to read for itself — a pipe belongs to the
// command line, and what reaches the library is the bytes it carried.
func source(name string) (vsc.Source, error) {
	if isStdout(name) {
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			return vsc.Source{}, fmt.Errorf("<stdin>: %w", err)
		}
		return vsc.Source{Name: "<stdin>", Text: src}, nil
	}
	src, err := os.ReadFile(name)
	if err != nil {
		return vsc.Source{}, err
	}
	return vsc.Source{Name: name, Text: src}, nil
}

// sources is source over a command line, in order. No names means
// standard input, so `vsc check < f.vs` works.
func sources(names []string) ([]vsc.Source, error) {
	if len(names) == 0 {
		names = []string{"-"}
	}
	out := make([]vsc.Source, 0, len(names))
	for _, name := range names {
		s, err := source(name)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// outputName is where an artifact lands when -o said nothing.
//
// The first input's base name with the right extension, which is what
// every compiler does and what a person expects: `vsc build fib.vs`
// writes `fib`, not `a.out`.
func outputName(srcs []vsc.Source, ext string) string {
	base := "a"
	if len(srcs) > 0 && srcs[0].Name != "<stdin>" {
		base = strings.TrimSuffix(filepath.Base(srcs[0].Name), filepath.Ext(srcs[0].Name))
	}
	return base + ext
}
