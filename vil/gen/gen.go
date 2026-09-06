package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// File lowers one checked file into a raw VIL module.
func File(name string, f *ast.File, info *analyzer.Info) (*vil.Module, []token.Diagnostic) {
	return Files(name, []*ast.File{f}, info)
}

// Files lowers a whole module: every file in it, into one VIL module.
//
// A module is the unit a symbol is named against and the unit access
// control is measured in, so the files are lowered together rather
// than one at a time and joined afterwards.
func Files(name string, files []*ast.File, info *analyzer.Info) (*vil.Module, []token.Diagnostic) {
	m := vil.NewModule(name, vil.StageRaw)
	m.Import("Builtin")

	// Which classes take part in inheritance has to be known before
	// any body is lowered: a call in the first file may be on a class
	// the last file subclasses.
	poly := polymorphic(files, info)

	var diags []token.Diagnostic
	for _, f := range files {
		g := &gen{m: m, info: info, file: f.Unit, module: name, poly: poly}
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			switch d := decl.D.(type) {
			case *ast.FuncDecl:
				g.function(d, nil)
			case *ast.StructDecl:
				g.members(d.Name, d.Body)
			case *ast.ClassDecl:
				g.members(d.Name, d.Body)
			case *ast.ActorDecl:
				g.members(d.Name, d.Body)
			case *ast.EnumDecl:
				g.members(d.Name, d.Body)
			}
		}
		diags = append(diags, g.diags...)
	}

	// The tables last, once every method has a symbol to point at.
	tg := &gen{m: m, info: info, module: name, poly: poly}
	if len(files) > 0 {
		tg.file = files[0].Unit
	}
	tg.vtables(files)
	diags = append(diags, tg.diags...)
	return m, diags
}

// polymorphic is the set of classes a method call on which cannot be
// bound statically: the ones with a superclass, and the ones that are
// somebody's superclass.
//
// A class in neither group has exactly one implementation of each of
// its methods and always will, so naming it directly is not an
// optimization but the only thing there is to name. For the rest,
// which body runs is a fact about the object rather than about the
// expression, and answering it here would answer it wrong -- see
// methodCall.
func polymorphic(files []*ast.File, info *analyzer.Info) map[*types.Class]bool {
	out := map[*types.Class]bool{}
	var mark func(t types.Type)
	mark = func(t types.Type) {
		if t == nil {
			return
		}
		if cl, ok := t.Underlying().(*types.Class); ok {
			out[cl] = true
		}
	}
	for _, f := range files {
		for _, stmt := range f.Stmts {
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			cd, ok := decl.D.(*ast.ClassDecl)
			if !ok || cd.Name == nil {
				continue
			}
			sym, _ := info.Defs[cd.Name].(*analyzer.TypeNameSymbol)
			if sym == nil {
				continue
			}
			cl, ok := sym.Type().Underlying().(*types.Class)
			if !ok || cl.Superclass == nil {
				continue
			}
			// The subclass, because a call on it may reach a body
			// declared below it; and the superclass, because a call
			// on it may reach one declared above.
			mark(cl)
			mark(cl.Superclass)
		}
	}
	return out
}

// members lowers the methods a type declares.
//
// A method is a function with the receiver as its last parameter, which
// is what the method convention says and what selfValue reads. Nothing
// else about it is special: the body is lowered by the same walk, and a
// name in it that turns out to be a stored property is reached through
// the receiver rather than through a local.
func (g *gen) members(name *ast.Ident, body *ast.MemberBlock) {
	if name == nil || body == nil {
		return
	}
	sym, _ := g.info.Uses[name].(*analyzer.TypeNameSymbol)
	if sym == nil {
		if def, ok := g.info.Defs[name].(*analyzer.TypeNameSymbol); ok {
			sym = def
		}
	}
	if sym == nil {
		return
	}
	for _, mem := range body.Members {
		if fn, ok := mem.(*ast.FuncDecl); ok {
			g.function(fn, sym.Type())
		}
	}
}

// symbol is the name a function is given in the module, which is the
// mangled one: SIL names a function by its symbol, and two functions
// that differ only in their types have to be told apart.
//
// A function this compiler cannot mangle yet has no symbol, and
// saying so is better than inventing one -- a symbol that is merely
// plausible links, and links to the wrong thing.
func (g *gen) symbol(sym *analyzer.FuncSymbol) string {
	// The entry point is the one symbol a linker looks for by name,
	// so it is the one function that is not mangled. See entry.go.
	if g.isEntry(sym) {
		return EntryName
	}
	// A function declared inside another is local to it, so two of
	// them with the same name in different enclosing functions are
	// two functions. Mangling only the name gave them one symbol, and
	// the second definition was appended to the first.
	if enc, ok := g.nested[sym]; ok {
		d := mangle.Decl{
			Module:        g.module,
			Name:          sym.Name(),
			Signature:     sym.Signature(),
			Discriminator: mangle.Discriminator(g.file.Name() + "\x00" + enc),
		}
		if name, err := mangle.Function(d); err == nil {
			return name
		}
	}
	d := mangle.Decl{
		Module:    g.module,
		Name:      sym.Name(),
		Signature: sym.Signature(),
	}
	// A private declaration is file-local, so its symbol says which
	// file, and two modules' worth of private helpers with the same
	// name stay apart.
	switch sym.Access() {
	case analyzer.Private, analyzer.FilePrivate:
		d.Discriminator = mangle.Discriminator(g.file.Name())
	}
	name, err := mangle.Function(d)
	if err != nil {
		g.diags = append(g.diags, token.Diagnostic{
			Pos:      sym.Pos(),
			End:      sym.Pos(),
			Severity: token.Error,
			Message:  "cannot name '" + sym.Name() + "': " + err.Error(),
		})
		return sym.Name()
	}
	return name
}

// linkageOf is the linkage an access level gives a symbol.
//
// The two ends of the range are what matter to a linker: a public
// symbol is one another module may resolve, and a private one may not
// leave the file it was written in. In between, internal and package
// are both hidden from outside the module -- SIL keeps them apart
// because a package is a set of modules built together and can share
// what a stranger cannot.
//
// `open` is `public` here. What it adds is about overriding rather
// than about visibility, and a linker has no opinion on overriding.
func linkageOf(a analyzer.Access) vil.Linkage {
	switch a {
	case analyzer.Public, analyzer.Open:
		return vil.Public
	case analyzer.Package:
		return vil.PackageLinkage
	case analyzer.Private, analyzer.FilePrivate:
		return vil.Private
	}
	return vil.Hidden
}

// A gen lowers one file.
type gen struct {
	m      *vil.Module
	info   *analyzer.Info
	file   *token.File
	module string

	fn     *vil.Func
	entry  bool // fn is the program's entry point
	blk    *vil.Block
	scopes []*scope
	locals map[analyzer.Symbol]*local

	// The loops break and continue can leave, innermost last, and the
	// label the next one will answer to.
	loops   []loop
	pending string

	// recv is the type whose method is being lowered, or nil in a free
	// function. A name that is not a local is looked for among its
	// stored properties.
	recv types.Type

	// closures counts the closure bodies lowered so far, so that each
	// gets a name of its own.
	closures int

	// poly is the classes whose methods cannot be bound statically.
	poly map[*types.Class]bool

	// nested maps a function declared inside another to the symbol of
	// the one it was declared in, which is what tells two local
	// functions of the same name apart.
	nested map[analyzer.Symbol]string

	diags []token.Diagnostic
}

// unsupported records an expression this package cannot lower.
//
// The alternative is what it used to do: return nothing, and let the
// statement above substitute the empty tuple, so a function declared
// to return Int returned Void and only the verifier noticed. Silence
// about what was not lowered is how a compiler emits a program that
// is not the one it was given.
func (g *gen) unsupported(e ast.Expr) {
	name := "an expression"
	if e != nil {
		name = g.exprKind(e)
	}
	g.refuse(e, name)
}

// unsupportedStmt records a statement this package cannot lower.
//
// A statement is the more dangerous of the two to drop, because
// nothing downstream can notice. An expression that produced no value
// leaves a hole something asks about; a `while` that was never
// lowered leaves a program that verifies, links, runs, and does not
// loop.
func (g *gen) unsupportedStmt(s ast.Stmt) {
	name := "a statement"
	if s != nil {
		name = stmtKind(s)
	}
	g.refuse(s, name)
}

// errorAt reports something wrong with the program, sited on the node
// that is wrong.
//
// It is separate from refuse because the two say different things. A
// refusal is about this compiler — the construct is fine and is not
// lowered yet — and reads with a "yet" in it. This is about the
// program, and no amount of finishing the compiler will make it
// compile.
func (g *gen) errorAt(n ast.Node, msg string) {
	pos, end := token.NoPos, token.NoPos
	if n != nil {
		pos, end = n.Pos(), n.End()
	}
	g.diags = append(g.diags, token.Diagnostic{
		Pos:      pos,
		End:      end,
		Severity: token.Error,
		Message:  msg,
	})
}

// refuse reports that a node was not lowered, sited on the whole of
// it — the span rather than its first byte, so the caret covers what
// was refused.
func (g *gen) refuse(n ast.Node, name string) {
	pos, end := token.NoPos, token.NoPos
	if n != nil {
		pos, end = n.Pos(), n.End()
	}
	g.diags = append(g.diags, token.Diagnostic{
		Pos:      pos,
		End:      end,
		Severity: token.Error,
		Message:  "cannot lower " + name + " yet",
	})
}

// stmtKind names a statement the way a person would say it.
func stmtKind(s ast.Stmt) string {
	switch s.(type) {
	case *ast.ForInStmt:
		return "a for-in loop"
	case *ast.WhileStmt:
		return "a while loop"
	case *ast.RepeatWhileStmt:
		return "a repeat-while loop"
	case *ast.SwitchStmt:
		return "a switch"
	case *ast.GuardStmt:
		return "a guard"
	case *ast.DeferStmt:
		return "a defer"
	case *ast.DoStmt:
		return "a do block"
	case *ast.ThrowStmt:
		return "a throw"
	case *ast.BreakStmt:
		return "a break"
	case *ast.ContinueStmt:
		return "a continue"
	case *ast.FallthroughStmt:
		return "a fallthrough"
	case *ast.LabeledStmt:
		return "a labelled statement"
	case *ast.YieldStmt:
		return "a yield"
	case *ast.DiscardStmt:
		return "a discard"
	case *ast.IfConfigStmt:
		return "a compiler directive"
	}
	return "this statement"
}

func (g *gen) exprKind(e ast.Expr) string {
	switch n := e.(type) {
	case *ast.PrefixExpr:
		return "this prefix operator"
	case *ast.PostfixExpr:
		return "a postfix operator"
	case *ast.ClosureExpr:
		return "a closure"
	case *ast.SubscriptExpr:
		return "a subscript"
	case *ast.TernaryExpr, *ast.ConditionalExpr:
		return "a conditional expression"
	case *ast.ArrayLit:
		return "an array literal"
	case *ast.DictLit:
		return "a dictionary literal"
	case *ast.TupleExpr:
		return "a tuple expression"
	case *ast.CallExpr:
		if id, ok := n.Fun.(*ast.IdentExpr); ok && id.Name != nil {
			// A name that is neither a function nor a value of
			// function type is a type, and calling one makes an
			// instance. Saying which reads very differently to
			// whoever wrote the call, so the two are told apart
			// rather than both called "this call".
			if _, isFunc := g.info.Uses[id.Name].(*analyzer.FuncSymbol); !isFunc {
				if _, callable := g.info.Types[id].Underlying().(*types.Signature); !callable {
					return "a constructor call"
				}
			}
		}
		return "this call"
	}
	return "this expression"
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
//
// Two kinds, and SILGen has both. A lexical scope is a block: what it
// declares lives until the braces close. A formal scope is one
// statement: the borrows an expression needed to be evaluated end
// when the statement that evaluated it is done, not when the block
// is. `let kept = b` borrows b to copy it, and that borrow is over
// once the binding exists — holding it to the end of the function
// would be a borrow nothing is reading.
type scope struct {
	cleanups []cleanup
	formal   bool
}

// A cleanup is one thing to emit on the way out: a value to destroy,
// a borrow to end, a box to release.
type cleanup struct {
	destroy   *vil.Value
	endBorrow *vil.Value
}

func (g *gen) push()       { g.scopes = append(g.scopes, &scope{}) }
func (g *gen) pushFormal() { g.scopes = append(g.scopes, &scope{formal: true}) }
func (g *gen) top() *scope { return g.scopes[len(g.scopes)-1] }

// lexical is the innermost scope that is not one statement's.
func (g *gen) lexical() *scope {
	for i := len(g.scopes) - 1; i >= 0; i-- {
		if !g.scopes[i].formal {
			return g.scopes[i]
		}
	}
	return g.scopes[0]
}

// destroyLater registers an owned value to be destroyed where its
// lexical scope ends — the block, not the statement. A value that
// lives in a name lives as long as the name does.
func (g *gen) destroyLater(v *vil.Value) {
	if v == nil || v.Ownership() != vil.Owned {
		return
	}
	s := g.lexical()
	s.cleanups = append(s.cleanups, cleanup{destroy: v})
}

// endBorrowLater registers a borrow to be closed where the statement
// that opened it ends.
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
func (g *gen) unwind() { g.unwindTo(0) }

// unwindTo is unwind as far as a depth and no further, which is what
// a break and a continue do: they leave the scopes inside the loop
// and not the ones the loop is inside.
//
// Like unwind, it emits rather than pops. The scopes are still open
// for whatever follows the branch, because a break is one path out of
// a block and not the end of it.
func (g *gen) unwindTo(depth int) {
	for i := len(g.scopes) - 1; i >= depth; i-- {
		g.emitCleanups(g.scopes[i])
	}
}

// A loop is somewhere break and continue can go.
//
// depth is how deep the scope stack was when the loop was entered, so
// that leaving it runs the cleanups of the scopes inside it and none
// of the ones outside.
type loop struct {
	header *vil.Block // continue: test the condition again
	exit   *vil.Block // break: the statement after the loop
	depth  int
	label  string // "" unless the loop was written with one

	// lazyExit stands in for exit where the block after the statement
	// may turn out never to be reached — a switch whose every case
	// returns has nothing after it, and a block nothing branches to is
	// one the verifier rejects. Calling it is what makes the block, so
	// it is called only by something that is about to branch there.
	lazyExit func() *vil.Block
}

// exitBlock is where a break inside this statement goes, making the
// block if it does not exist yet.
func (l loop) exitBlock() *vil.Block {
	if l.exit != nil {
		return l.exit
	}
	if l.lazyExit != nil {
		return l.lazyExit()
	}
	return nil
}

// enclosing is the loop a break or a continue names: the innermost
// one, or the one with that label.
func (g *gen) enclosing(label string) (loop, bool) {
	for i := len(g.loops) - 1; i >= 0; i-- {
		if label == "" || g.loops[i].label == label {
			return g.loops[i], true
		}
	}
	return loop{}, false
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
// recv is the type the function is a method of, or nil for a free
// function.
func (g *gen) function(d *ast.FuncDecl, recv types.Type) {
	sym, _ := g.info.Defs[d.Name].(*analyzer.FuncSymbol)
	if sym == nil {
		return
	}
	sig := sym.Signature()
	g.recv = recv

	// The entry point is named, called and returned from differently
	// from every other function, and a main this compiler cannot use
	// as one is refused here rather than left to the linker.
	g.entry = g.isEntry(sym)
	if g.entry && !g.checkEntry(sym, sig) {
		g.entry = false
		return
	}

	name := g.symbol(sym)
	if recv != nil {
		name = g.methodSymbol(&analyzer.MethodRef{
			Recv:   recv,
			Method: &types.Method{Name: sym.Name(), Sig: sig},
		})
	}

	linkage := linkageOf(sym.Access())
	if g.entry {
		// A program's entry point leaves its object file whatever the
		// source said about who may call it, because what resolves it
		// is the linker rather than another module.
		linkage = vil.Public
	}
	f := g.m.Func(name).SetSourceName(sym.Name()).
		SetLinkage(linkage).SetAttr("ossa")
	g.fn = f
	// Every function declared inside this one is registered before any
	// of the body is lowered, so that a call finds the same symbol
	// wherever in the body it appears -- including from a sibling
	// nested function, where the enclosing function is no longer the
	// one being lowered.
	g.registerNested(d.Body, name)
	// The definition states the type; a forward declaration only
	// guessed at it. A call written above the declaration it names
	// creates the function and fills its parameter list in, and
	// appending to that list here gave the function twice as many
	// parameters as its entry block had arguments -- which the
	// verifier caught, and which made calling a function declared
	// further down the file impossible.
	f.Type().Params = nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes = nil
	g.loops, g.pending = nil, ""
	g.push()
	g.blk = f.Entry()

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
			g.blk.DebugValue(v, name, "let", "argno "+itoa(i+1))
		}
		// An @owned parameter belongs to the callee, so the callee
		// releases it. A @guaranteed one belongs to the caller and is
		// alive for the whole call, which is what its convention
		// promises and why it needs no cleanup here.
		g.destroyLater(v)
	}
	if recv != nil {
		// Self last, which is the method convention's shape and what
		// selfValue reads. A struct receiver is a value; a class
		// receiver is a reference the caller keeps alive across the
		// call, which is @guaranteed.
		t := lowerType(recv)
		f.Param(t, selfConvention(t))
		f.Type().Convention = vil.Method
	}

	switch {
	case g.entry:
		entrySignature(f)
	case sig.Results != nil && !isVoid(sig.Results):
		f.SetResult(lowerType(sig.Results), resultConvention(lowerType(sig.Results)))
	}

	if d.Body != nil {
		g.block(d.Body)
	}
	// A body that falls off the end returns nothing, which only a
	// void function may do — and the checker already said so.
	//
	// A function with a result can still end up here with nowhere to
	// fall from: `switch` leaves a continuation block behind, and when
	// every case returns, nothing branches to it. Swift ends such a
	// block with `unreachable`, and so does this — inventing a value
	// to return would be a value the program could not have computed,
	// and returning void would be the wrong type.
	if g.blk != nil && g.blk.Term() == nil {
		g.unwind()
		if !g.entry && sig.Results != nil && !isVoid(sig.Results) {
			g.blk.Unreachable()
		} else {
			g.blk.Return(g.result())
		}
	}
	g.fn = nil
	g.entry = false
	g.recv = nil
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
