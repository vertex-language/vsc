package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// EntryModule is the module whose `main` is the program's entry point,
// and EntryName is the symbol it is given.
const (
	EntryModule = "main"
	EntryName   = "main"
)

// entryResult is what the entry point returns to the operating
// system: a process exit status, which is an int on every platform
// this compiler targets and Int32 in the type model.
func entryResult() types.Type { return types.Typ[types.Int32] }

// isEntry reports whether sym declares the program's entry point.
func (g *gen) isEntry(sym *analyzer.FuncSymbol) bool {
	return !g.script && g.module == EntryModule && sym.Name() == EntryName
}

// checkEntry reports whether the entry point matches a valid signature (func main() or func main() -> Int32).
func (g *gen) checkEntry(sym *analyzer.FuncSymbol, sig *types.Signature) bool {
	var why string
	switch {
	case len(sig.TypeParams) > 0:
		why = "is generic"
	case len(sig.Params) > 0:
		why = "takes parameters"
	case sig.Results != nil && !isVoid(sig.Results) && !isEntryResult(sig.Results):
		why = "returns " + sig.Results.String()
	default:
		return true
	}
	g.diags = append(g.diags, token.Diagnostic{
		Pos:      sym.Pos(),
		End:      sym.Pos(),
		Severity: token.Error,
		Message: "'main' is the entry point of module '" + EntryModule +
			"' and " + why + ": write 'func main()' or 'func main() -> Int32'",
	})
	return false
}

// isEntryResult reports whether t is the exit status type.
func isEntryResult(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.Int32
}

// entrySignature configures f with C convention returning Int32.
func entrySignature(f *sil.Func) {
	f.Type().Convention = sil.C
	f.SetResult(lowerType(entryResult()), sil.ResultUnowned)
}

// entryStatus returns the default exit status (Int32(0)) for main.
func (g *gen) entryStatus() *sil.Value {
	raw := g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt32), 0)
	return g.blk.Struct(lowerType(entryResult()), raw)
}

// result returns the return value for an empty return statement (void tuple or entry status).
func (g *gen) result() *sil.Value {
	if g.entry {
		return g.entryStatus()
	}
	return g.void()
}

// asyncThrowingEntry is the entry for a main that is async and throws:
// its body is wrapped in an async function that catches what it throws
// and ends the program with it, and that function is run as an async
// main is. It is the wrapper.
func (g *gen) asyncThrowingEntry(body *sil.Func) *sil.Func {
	f := g.m.Func(asyncEntryName + "_caught").SetSourceName(EntryName).SetLinkage(sil.Hidden).SetAttr("ossa")
	f.Type().Async = true
	f.Type().Params = nil
	f.SetResult(lowerType(entryResult()), sil.ResultUnowned)

	prevFn, prevBlk, prevScopes := g.fn, g.blk, g.scopes
	defer func() { g.fn, g.blk, g.scopes = prevFn, prevBlk, prevScopes }()
	g.fn, g.blk, g.scopes = f, f.Entry(), nil
	g.push()

	normal, failed := f.Block(), f.Block()
	var status *sil.Value
	if len(body.Type().Results) == 1 {
		status = normal.Arg(lowerType(entryResult()), sil.Owned)
	}
	box := failed.Arg(errorBoxType(), sil.Owned)
	g.blk.TryApply(g.blk.FunctionRef(body), normal, failed)

	g.blk = failed
	g.runtimeResult(stdlib.ErrorInMain,
		[]sil.Param{{Type: errorBoxType(), Convention: sil.ParamOwned}},
		lowerType(types.Typ[types.Void]), box)
	g.blk.Unreachable()

	g.blk = normal
	g.unwind()
	if status == nil {
		status = g.entryStatus()
	}
	g.blk.Return(status)
	return f
}

// throwingEntryName is the symbol a throwing main's body is given, the
// name main being the entry that calls it.
const throwingEntryName = "$vsc_throwing_main"

// throwingEntry emits the program's entry for a main that throws: it calls
// body, exits with what body returns (or 0), and hands an error body throws
// to the runtime, which reports it and traps -- what Swift does with an
// error raised at top level.
func (g *gen) throwingEntry(name string, body *sil.Func) {
	f := g.m.Func(name).SetSourceName(EntryName).SetLinkage(sil.Public).SetAttr("ossa")
	entrySignature(f)

	prevFn, prevBlk, prevScopes := g.fn, g.blk, g.scopes
	defer func() { g.fn, g.blk, g.scopes = prevFn, prevBlk, prevScopes }()
	g.fn, g.blk, g.scopes = f, f.Entry(), nil
	g.push()

	normal, failed := f.Block(), f.Block()
	var status *sil.Value
	if len(body.Type().Results) == 1 {
		status = normal.Arg(lowerType(entryResult()), sil.Owned)
	}
	box := failed.Arg(errorBoxType(), sil.Owned)
	g.blk.TryApply(g.blk.FunctionRef(body), normal, failed)

	g.blk = failed
	g.runtimeResult(stdlib.ErrorInMain,
		[]sil.Param{{Type: errorBoxType(), Convention: sil.ParamOwned}},
		lowerType(types.Typ[types.Void]), box)
	g.blk.Unreachable()

	g.blk = normal
	g.unwind()
	if status == nil {
		status = g.entryStatus()
	}
	g.blk.Return(status)
}
