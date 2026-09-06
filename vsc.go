package vsc

import (
	"errors"
	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/lower"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/gen"
	"github.com/vertex-language/vsc/vil/pass"
)

// A Diagnostic is one message and the file it is about.
//
// token.Diagnostic carries a position but not a file, because a
// token.File owns a position space that starts at zero and knows
// nothing of any other -- so a position alone does not say which file
// it is in. Pairing them here is what lets a caller print a
// diagnostic from a compilation of several files.
//
// File is nil for a failure that is about the module rather than
// about a line: a pass refusing a program, or lowering refusing an
// instruction.
type Diagnostic struct {
	token.Diagnostic
	File *token.File
}

func (d Diagnostic) String() string {
	if d.File == nil {
		return d.Message
	}
	return d.Print(d.File)
}

// Errors reports whether any of these stop a compilation.
func Errors(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == token.Error {
			return true
		}
	}
	return false
}

// A Source is one file to compile.
type Source struct {
	Name string
	Text []byte
}

// Options are what the caller decides.
type Options struct {
	// Module is the name the module is compiled as. It is part of
	// every symbol, so two modules that share a name share symbols.
	Module string
	// Target is the machine to lower for.
	Target ir.Target
	// Stop says which phase to stop after. The zero value runs them
	// all.
	Stop Phase
}

// A Phase is one step of the compiler, in the order they run.
type Phase int

const (
	// All runs every phase. It is the zero value because running the
	// whole compiler is what a caller usually wants.
	All Phase = iota
	// Parsed stops after the syntax tree.
	Parsed
	// Checked stops after names and types.
	Checked
	// Raw stops after the ownership IR is generated.
	Raw
	// Canonical stops after the passes that must run.
	Canonical
	// Lowered stops after ownership is erased.
	Lowered
)

// A Unit is what the phases produced. A field is nil where its phase
// did not run, either because the caller stopped before it or because
// a diagnostic did.
type Unit struct {
	// Files are the parsed files, in the order they were given.
	Files []*ast.File
	// Positions maps each file to the position table it was scanned
	// against, which is what turns a diagnostic into a line and
	// column.
	Positions []*token.File
	// Info is what the checker learned.
	Info *analyzer.Info
	// VIL is the ownership IR. Its stage says how far the passes got.
	VIL *vil.Module
	// VIR is the machine IR.
	VIR *ir.Module
}

// Compile runs the phases in order and stops at the first one that
// reports an error.
//
// Diagnostics are returned rather than printed, and a warning does not
// stop anything: it is the caller's business how to show them and
// whether to care. What stops the compiler is an error, and it stops
// at the end of the phase that found it rather than at the first one
// -- so a file with three type errors reports three, and does not
// report the consequences of the first in the phase after.
func Compile(srcs []Source, opts Options) (*Unit, []Diagnostic) {
	u := &Unit{}
	var diags []Diagnostic

	for _, src := range srcs {
		tf := token.NewFile(src.Name, src.Text)
		file, ds := parser.ParseFile(tf, 0)
		u.Files = append(u.Files, file)
		u.Positions = append(u.Positions, tf)
		diags = append(diags, attribute(ds, tf)...)
	}
	if opts.Stop == Parsed || Errors(diags) {
		return u, diags
	}

	// The checker and the generator are given every file at once and
	// report against a position, and a position does not say which
	// file. Where there is one file there is no ambiguity; where there
	// are several, the message is still right and the line is not, so
	// it is left unattributed rather than attributed wrongly.
	info, checks := analyzer.Check(u.Files)
	u.Info = info
	diags = append(diags, attribute(checks, u.only())...)
	if opts.Stop == Checked || Errors(diags) {
		return u, diags
	}

	m, gens := gen.Files(opts.Module, u.Files, info)
	u.VIL = m
	diags = append(diags, attribute(gens, u.only())...)
	if opts.Stop == Raw || Errors(diags) {
		return u, diags
	}

	if err := pass.Mandatory(m); err != nil {
		return u, append(diags, phaseError(err))
	}
	if opts.Stop == Canonical {
		return u, diags
	}

	if err := pass.LowerOwnership(m); err != nil {
		return u, append(diags, phaseError(err))
	}
	if opts.Stop == Lowered {
		return u, diags
	}

	// Lowering needs a machine, and the zero Target is not one: it
	// describes nothing, and lowering against it produces diagnostics
	// about the program that are really diagnostics about the caller.
	// Saying so is worth more than the phase it saves.
	if !opts.Target.Valid() {
		return u, append(diags, phaseError(errNoTarget))
	}
	out, err := lower.Module(m, opts.Target, lower.Options{
		SymbolPrefix: SymbolPrefix(opts.Target),
	})
	if err != nil {
		return u, append(diags, phaseError(err))
	}
	u.VIR = out
	return u, diags
}

// only is the one file this unit is of, or nil where there are
// several and a position cannot say which.
func (u *Unit) only() *token.File {
	if len(u.Positions) == 1 {
		return u.Positions[0]
	}
	return nil
}

// attribute pairs each diagnostic with the file it is about.
func attribute(ds []token.Diagnostic, f *token.File) []Diagnostic {
	out := make([]Diagnostic, len(ds))
	for i, d := range ds {
		out[i] = Diagnostic{Diagnostic: d, File: f}
	}
	return out
}

// phaseError turns a failure with no position -- one a pass or the
// lowering reported about the module rather than about a line -- into
// a diagnostic, so that a caller has one kind of thing to report.
func phaseError(err error) Diagnostic {
	return Diagnostic{Diagnostic: token.Diagnostic{
		Pos:      token.NoPos,
		End:      token.NoPos,
		Severity: token.Error,
		Message:  err.Error(),
	}}
}

// errNoTarget is what Compile reports when it is asked to lower for
// no machine. Stopping at Canonical or earlier needs no target and
// does not reach this.
var errNoTarget = errors.New("no target: lowering needs a machine to lower for")
