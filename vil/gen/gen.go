// Package gen lowers a checked tree into raw VIL.
//
// SILGen's job, and SILGen's name for it. The analyzer decided what
// the program means; this decides what it does — which accessor a
// property reference calls, where a temporary lives, when a value is
// copied and where its lifetime ends. It rejects nothing: a program
// that reaches here was already found legal, and what this produces
// is raw VIL for vil/pass to check and vil/lower to translate.
//
// # Ownership is emitted, not inferred
//
// Every copy and every destroy is written down here. A `let` that
// binds a class reference copies it and destroys it where its scope
// ends; a member read borrows the base for exactly as long as the
// read takes. That is what makes the output verifiable the moment it
// exists: vil/verify checks the two rules against what this emitted,
// rather than against what a later pass hopes to work out.
//
// The mechanism is a stack of scopes. Entering one pushes; leaving
// one emits the cleanups it collected, in reverse; a return unwinds
// every scope on the way out. It is SILGen's cleanup stack, smaller.
//
// # What is lowered
//
// Functions, their parameters and results. Local `let` and `var`
// bindings. Member reads and writes on structs and classes.
// Assignment. `if`, `else`, and `return`. Calls to functions the
// checker resolved.
//
// What is not, and why: anything that needs a standard library. An
// integer literal in Swift is a call to `Int.init(_builtinIntegerLiteral:)`,
// and until core/ declares that, this emits the builtin literal
// directly and the output is one apply short of what swiftc prints.
// Closures, enums with payloads, existentials, generics and throwing
// wait on the same thing.
package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// File lowers one checked file into a raw VIL module.
func File(name string, f *ast.File, info *analyzer.Info) *vil.Module {
	m := vil.NewModule(name, vil.StageRaw)
	m.Import("Builtin")

	g := &gen{m: m, info: info, file: f.Unit}
	for _, stmt := range f.Stmts {
		decl, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		if fn, ok := decl.D.(*ast.FuncDecl); ok {
			g.function(fn)
		}
	}
	return m
}

// A gen lowers one file.
type gen struct {
	m    *vil.Module
	info *analyzer.Info
	file *token.File

	fn     *vil.Func
	blk    *vil.Block
	scopes []*scope
	locals map[analyzer.Symbol]*local
}

// A local is what a name in scope lowers to: a value, or the address
// of one.
//
// A `let` is a value — SILGen keeps it in SSA and moves it, which is
// what `move_value [var_decl]` says. A `var` is a box: allocated,
// borrowed for the variable's lifetime, and projected to get at what
// it holds, because a variable can be written and an SSA value
// cannot.
type local struct {
	value *vil.Value // a let: the value itself
	addr  *vil.Value // a var: the address inside its box
	box   *vil.Value // a var: the box
	typ   vil.Type
}

// A scope collects what has to be undone when it ends.
type scope struct {
	cleanups []cleanup
}

// A cleanup is one thing to emit on the way out: a value to destroy,
// a borrow to end, a box to release.
type cleanup struct {
	destroy   *vil.Value
	endBorrow *vil.Value
}

func (g *gen) push()       { g.scopes = append(g.scopes, &scope{}) }
func (g *gen) top() *scope { return g.scopes[len(g.scopes)-1] }

// destroyLater registers an owned value to be destroyed where the
// current scope ends.
func (g *gen) destroyLater(v *vil.Value) {
	if v == nil || v.Ownership() != vil.Owned {
		return
	}
	s := g.top()
	s.cleanups = append(s.cleanups, cleanup{destroy: v})
}

// endBorrowLater registers a borrow to be closed where the current
// scope ends.
func (g *gen) endBorrowLater(v *vil.Value) {
	s := g.top()
	s.cleanups = append(s.cleanups, cleanup{endBorrow: v})
}

// forget drops a pending cleanup for a value whose ownership is being
// handed on — returned, stored, or bound to a name that will destroy
// it instead. Whoever takes it owes the consume now, and leaving the
// cleanup behind would consume it twice.
func (g *gen) forget(v *vil.Value) {
	if v == nil {
		return
	}
	for _, s := range g.scopes {
		for i, c := range s.cleanups {
			if c.destroy == v {
				s.cleanups = append(s.cleanups[:i], s.cleanups[i+1:]...)
				return
			}
		}
	}
}

// pop emits the current scope's cleanups and leaves it.
func (g *gen) pop() {
	g.emitCleanups(g.top())
	g.scopes = g.scopes[:len(g.scopes)-1]
}

// unwind emits every open scope's cleanups without leaving them,
// which is what a return does: the scopes are still there for the
// code after the branch that did not return.
func (g *gen) unwind() {
	for i := len(g.scopes) - 1; i >= 0; i-- {
		g.emitCleanups(g.scopes[i])
	}
}

// emitCleanups writes one scope's undoing, in reverse order of
// declaration — a value is destroyed before the thing it was made
// from.
func (g *gen) emitCleanups(s *scope) {
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		switch c := s.cleanups[i]; {
		case c.endBorrow != nil:
			g.blk.EndBorrow(c.endBorrow)
		case c.destroy != nil:
			g.blk.DestroyValue(c.destroy)
		}
	}
}

// function lowers one declaration.
func (g *gen) function(d *ast.FuncDecl) {
	name := d.Name.Text(g.file)
	sym, _ := g.info.Defs[d.Name].(*analyzer.FuncSymbol)
	if sym == nil {
		return
	}
	sig := sym.Signature()

	f := g.m.Func(name).SetLinkage(vil.Hidden).SetAttr("ossa")
	g.fn = f
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.push()

	// The parameters, in order, with the conventions their types and
	// their ownership give them.
	params := paramSymbols(d, g.info, g.file)
	for i, p := range sig.Params {
		t := lowerType(p.Type)
		conv := paramConvention(p, t)
		v := f.Param(t, conv)
		if i < len(params) && params[i] != nil {
			g.locals[params[i]] = &local{value: v, typ: t}
		}
		if name := p.Name; name != "" {
			g.blockOf(f).DebugValue(v, name, "let", "argno "+itoa(i+1))
		}
	}
	if sig.Results != nil && !isVoid(sig.Results) {
		f.SetResult(lowerType(sig.Results), resultConvention(lowerType(sig.Results)))
	}

	g.blk = f.Entry()
	if d.Body != nil {
		g.block(d.Body)
	}
	// A body that falls off the end returns nothing, which only a
	// void function may do — and the checker already said so.
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		g.blk.Return(g.void())
	}
	g.fn = nil
}

// blockOf is the block instructions go into while a function's
// parameters are being declared.
func (g *gen) blockOf(f *vil.Func) *vil.Block {
	g.blk = f.Entry()
	return g.blk
}

// void is the empty tuple every function without a result returns.
func (g *gen) void() *vil.Value {
	return g.blk.Tuple(vil.Object(types.Typ[types.Void]))
}

func isVoid(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.Void
}

// paramSymbols is the symbol each parameter binds, so that a use of
// the name inside the body finds the value.
func paramSymbols(d *ast.FuncDecl, info *analyzer.Info, f *token.File) []analyzer.Symbol {
	if d.Sig == nil {
		return nil
	}
	out := make([]analyzer.Symbol, 0, len(d.Sig.Params))
	for range d.Sig.Params {
		out = append(out, nil)
	}
	// The analyzer records a parameter's symbol under the function's
	// scope; the body's uses resolve to it, and matching by position
	// is enough because a signature's parameters are in order.
	if scope := info.Scopes[d]; scope != nil {
		for i, p := range d.Sig.Params {
			name := p.Name
			if name == nil {
				name = p.Label
			}
			if name == nil {
				continue
			}
			if sym := scope.Lookup(name.Text(f)); sym != nil {
				out[i] = sym
			}
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
