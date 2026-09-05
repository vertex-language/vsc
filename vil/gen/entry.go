package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// The entry point: the one function a linker finds by name.
//
// Every other function in a module carries a mangled symbol, because
// two that differ only in their types have to be told apart. The
// entry point is the exception and has to be: which symbol a program
// starts at is decided by the platform rather than by the compiler,
// and what the platform decided is `main`.
//
// Swift arrives at the same symbol from the other end. Top-level code
// is collected into a function SILGen names `@main`, mangled not at
// all and called the way C is called:
//
//	sil [ossa] @main : $@convention(c) (Int32, UnsafeMutablePointer<...>) -> Int32
//
// This takes the Vertex spelling — a declared `func main()` — and
// gives it that name and that convention. Two differences from
// Swift's, both deliberate. It takes no parameters, because argc and
// argv are what a runtime hands a program and there is no runtime to
// hand them over yet. And it comes from a declaration rather than
// from top-level code, which is the Vertex rule: a program says what
// it starts with instead of accumulating it.
//
// Which module is the program is the caller's to say. Options.Module
// is `main` for a program and the library's own name otherwise, so a
// library that happens to declare a helper called main keeps its
// mangled symbol, and no two objects ever fight over `_main`.

// EntryModule is the module whose `main` is the program's entry
// point, and EntryName is the symbol it is given. The rule is Go's,
// for Go's reason: something has to say which of the modules being
// compiled is the program, and a name costs no syntax.
const (
	EntryModule = "main"
	EntryName   = "main"
)

// entryResult is what the entry point returns to the operating
// system: a process exit status, which is an int on every platform
// this compiler targets and Int32 in the type model.
func entryResult() types.Type { return types.Typ[types.Int32] }

// isEntry reports whether sym declares the program's entry point.
//
// Only the name and the module are asked here. Whether the
// declaration is one this compiler can use as an entry point is
// checkEntry's question, and it is asked second so that a `main` with
// the wrong shape is a diagnostic about the entry point rather than
// an ordinary function with a surprising symbol.
func (g *gen) isEntry(sym *analyzer.FuncSymbol) bool {
	return g.module == EntryModule && sym.Name() == EntryName
}

// checkEntry reports whether the entry point is declared in one of
// the two shapes there are, and says so where it is not.
//
// The shapes are `func main()`, which exits zero, and `func main() ->
// Int32`, which exits with what it returns. Anything else is
// refused rather than mangled and left as an ordinary function: a
// program whose main is not the entry point links to nothing, and
// finding that out from the linker is finding it out three phases too
// late.
func (g *gen) checkEntry(sym *analyzer.FuncSymbol, sig *types.Signature) bool {
	var why string
	switch {
	case len(sig.TypeParams) > 0:
		why = "is generic"
	case len(sig.Params) > 0:
		why = "takes parameters"
	case sig.Async:
		why = "is async"
	case sig.Throws:
		why = "throws"
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

// entrySignature gives f the type the platform calls: C convention,
// no parameters, an Int32 out.
//
// The result is set here even for a `func main()` that names none,
// because what the source says it returns and what the process
// returns are different questions. A main that returns nothing still
// exits with a status, and the status is zero.
func entrySignature(f *vil.Func) {
	f.Type().Convention = vil.C
	f.SetResult(lowerType(entryResult()), vil.ResultUnowned)
}

// entryStatus is the exit status a body that named none returns: the
// zero an implicit `return` at the end of main means.
//
//	%0 = integer_literal $Builtin.Int32, 0
//	%1 = struct $Int32 (%0)
//	return %1
//
// Which is what SILGen writes at the end of Swift's own `@main`.
func (g *gen) entryStatus() *vil.Value {
	raw := g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt32), 0)
	return g.blk.Struct(lowerType(entryResult()), raw)
}

// result is the value a return with nothing to return returns.
//
// Every function's is the empty tuple, except the entry point's,
// whose type says Int32 whatever its declaration said.
func (g *gen) result() *vil.Value {
	if g.entry {
		return g.entryStatus()
	}
	return g.void()
}
