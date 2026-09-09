package cli

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/vertex-language/vsc/iface"
	"io"

	"github.com/vertex-language/ir"
	irtext "github.com/vertex-language/ir/text"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/vil/text"
)

// The emit modes: how far down to go, and what to write.
//
// Each one names the phase it stops at, so a mode can never ask for
// an artifact a phase did not produce. `exe` is the default because a
// compiler with no mode flag should build a program.
type emitMode struct {
	name string
	stop vsc.Phase
	ext  string
}

var emits = []emitMode{
	{"exe", vsc.All, ""},
	{"obj", vsc.All, ".o"},
	{"vir", vsc.All, ".vir"},
	{"vil", vsc.Lowered, ".sil"},
	// The module's public face, which is what another module is
	// compiled against. It needs no machine, so it stops at the
	// checker.
	{"interface", vsc.Checked, iface.Extension},
}

func lookupEmit(name string) (emitMode, bool) {
	for _, e := range emits {
		if e.name == name {
			return e, true
		}
	}
	return emitMode{}, false
}

func emitNames() string {
	var b bytes.Buffer
	for i, e := range emits {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(e.name)
	}
	return b.String()
}

// buildFlags are build's and run's, since run is build followed by
// starting what it built.
type buildFlags struct {
	common
	emit         string
	output       string
	entry        string
	freestanding bool
}

func (b *buildFlags) register(fs *flag.FlagSet) {
	b.common.register(fs)
	fs.StringVar(&b.emit, "emit", "exe", "what to produce: "+emitNames())
	fs.StringVar(&b.output, "o", "", "write output here (\"-\" is standard output)")
	fs.StringVar(&b.entry, "entry", "", "the program's entry symbol (default: the platform's)")
	fs.BoolVar(&b.freestanding, "freestanding", false, "link no platform libraries")
}

func cmdBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var bf buildFlags
	bf.register(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	_, code := doBuild(&bf, fs.Args(), stdout, stderr)
	return code
}

// doBuild is the whole of build, and the first half of run: it
// returns where the artifact landed so that run can start it.
func doBuild(bf *buildFlags, names []string, stdout, stderr io.Writer) (string, int) {
	mode, ok := lookupEmit(bf.emit)
	if !ok {
		fmt.Fprintf(stderr, "vsc: unknown --emit %q (known: %s)\n", bf.emit, emitNames())
		return "", exitUsage
	}
	target, err := bf.resolve()
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	srcs, err := sources(names)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}

	u, diags := vsc.Compile(srcs, bf.options(target, mode.stop))
	if printDiags(stderr, diags) {
		return "", exitDiags
	}

	out := bf.output
	if out == "" {
		out = outputName(srcs, mode.ext)
	}

	switch mode.name {
	case "interface":
		var buf bytes.Buffer
		if err := iface.Print(&buf, iface.Module{
			Name:  bf.module,
			Files: u.Files,
			Units: u.Positions,
			Info:  u.Info,
		}); err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, buf.Bytes(), false)

	case "vil":
		var buf bytes.Buffer
		if err := text.Print(&buf, u.VIL); err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, buf.Bytes(), false)

	case "vir":
		var buf bytes.Buffer
		if err := irtext.Print(&buf, u.VIR); err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, buf.Bytes(), false)

	case "obj":
		obj, err := object(u.VIR)
		if err != nil {
			fmt.Fprintln(stderr, "vsc:", err)
			return "", exitUsage
		}
		return out, write(out, stdout, stderr, obj, false)
	}

	// exe: the object, then the link.
	obj, err := object(u.VIR)
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	entry := bf.entry
	if entry == "" {
		entry = vsc.EntrySymbol(target)
	}
	exe, err := build.Executable([]build.Input{{Name: outputName(srcs, ".o"), Data: obj}},
		build.LinkOptions{
			Target:       target,
			Entry:        entry,
			Freestanding: bf.freestanding,
		})
	if err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return "", exitUsage
	}
	return out, write(out, stdout, stderr, exe, true)
}

func object(m *ir.Module) ([]byte, error) { return build.Object(m, build.Options{}) }

func write(name string, stdout, stderr io.Writer, data []byte, exec bool) int {
	if err := writeOut(name, stdout, data, exec); err != nil {
		fmt.Fprintln(stderr, "vsc:", err)
		return exitUsage
	}
	return exitOK
}
