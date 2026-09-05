package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Expressions.
//
// Every one produces a value the caller may use, and every one leaves
// the ownership right: a borrow is opened and closed around the use
// that needed it, and anything owned that is produced here is
// registered with the enclosing scope so that it is destroyed exactly
// once.

// expr lowers an expression to a value.
func (g *gen) expr(e ast.Expr) *vil.Value {
	switch n := e.(type) {
	case *ast.BasicLit:
		return g.literal(n)

	case *ast.IdentExpr:
		return g.ident(n)

	case *ast.MemberExpr:
		return g.member(n)

	// `.red` where a Color is wanted. The checker resolved the base
	// and recorded the case under the name, so there is nothing left
	// to look up: it lowers exactly as `Color.red` does.
	case *ast.ImplicitMemberExpr:
		if n.Name == nil {
			return nil
		}
		ec, ok := g.info.Uses[n.Name].(*analyzer.EnumCaseSymbol)
		if !ok {
			g.refuse(n, "this implicit member")
			return nil
		}
		return g.enumCase(n, ec)

	case *ast.ClosureExpr:
		return g.closure(n)

	case *ast.CallExpr:
		return g.call(n)

	case *ast.ParenExpr:
		return g.expr(n.X)

	case *ast.SelfExpr:
		return g.selfValue()

	case *ast.SequenceExpr:
		// The analyzer folded it; the folded tree is what runs.
		if folded, ok := g.info.Folded[n]; ok {
			return g.expr(folded)
		}

	case *ast.BinaryExpr:
		return g.binary(n)

	case *ast.ConditionalExpr:
		return g.conditional(n)

	case *ast.PrefixExpr:
		return g.prefix(n)
	}
	g.unsupported(e)
	return nil
}

// prefix lowers a prefix operator. It is not always one instruction:
// Swift writes negation as a subtraction from zero and bitwise
// inversion as two of them, and core says which, so that what is
// emitted here is what `swiftc -emit-sil` emits for the same source.
func (g *gen) prefix(e *ast.PrefixExpr) *vil.Value {
	sym, _ := g.info.Operators[e].(*analyzer.FuncSymbol)
	if sym == nil {
		g.expr(e.X)
		g.unsupported(e)
		return nil
	}
	operand := g.info.Types[e.X]
	steps, ok := core.LowerPrefix(sym.Name(), operand)
	if !ok {
		g.expr(e.X)
		g.unsupported(e)
		return nil
	}
	v := g.expr(e.X)
	if v == nil {
		return nil
	}
	result := lowerType(sym.Signature().Results)
	if len(steps) == 0 {
		// Unary plus is the operand.
		return v
	}

	// An expansion of more than one step reuses the literal that says
	// whether to report, which is what SILGen's own inlining leaves
	// behind and so what the diff is taken against.
	flags := make(map[int64]*vil.Value, 1)
	cur := g.machine(v, operand)
	for _, st := range steps {
		cur = g.step(st, cur, flags)
		if cur == nil {
			return nil
		}
	}
	return g.blk.Struct(result, cur)
}

// step emits one builtin of a prefix operator's expansion.
func (g *gen) step(st core.Step, operand *vil.Value, flags map[int64]*vil.Value) *vil.Value {
	out := vil.Object(builtinNamed(st.Result))
	args := []*vil.Value{operand}
	if st.HasConst {
		k := g.blk.IntegerLiteral(vil.Object(builtinNamed(st.Result)), st.Const)
		if st.ConstLeft {
			args = []*vil.Value{k, operand}
		} else {
			args = append(args, k)
		}
	}
	if !st.Overflows {
		return g.blk.Builtin(st.Name, out, args...)
	}

	// The last operand of a reporting builtin says whether to report:
	// all bits set for yes, none for the wrapping arithmetic that `~`
	// is made of.
	want := int64(0)
	if st.Reports {
		want = -1
	}
	flag, ok := flags[want]
	if !ok {
		flag = g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), want)
		flags[want] = flag
	}
	args = append(args, flag)
	pair := vil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinNamed(st.Result)},
		{Type: vil.BuiltinInt1},
	}})
	both := g.blk.Builtin(st.Name, pair, args...)
	value := g.blk.TupleExtract(both, 0, out)
	if st.Reports {
		flag := g.blk.TupleExtract(both, 1, vil.Object(vil.BuiltinInt1))
		g.blk.CondFail(flag, "arithmetic overflow")
	}
	return value
}

// rvalue is an expression lowered where its ownership is about to be
// handed on: stored, bound to a name, returned, or passed to
// something that takes it.
//
// Two things follow from that. A borrowed value is copied, because
// what is borrowed cannot be given away. An owned one has its pending
// cleanup forgotten, because the consume is now the receiver's to
// make and making it twice is the fault the verifier names.
func (g *gen) rvalue(e ast.Expr) *vil.Value {
	v := g.expr(e)
	if v == nil {
		return nil
	}
	if v.Ownership() == vil.Guaranteed {
		return g.blk.CopyValue(v)
	}
	g.forget(v)
	return v
}

// literal lowers a literal to the builtin it is made of, wrapped in
// the type that holds it.
//
// SILGen writes something else: an `integer_literal
// $Builtin.IntLiteral` and an apply of
// `Int.init(_builtinIntegerLiteral:)`, because in Swift a literal is
// a call to an initializer a library declares. That initializer is
// [transparent], so the first mandatory pass inlines it and canonical
// SIL reads exactly what this emits:
//
//	%0 = integer_literal $Builtin.Int64, 0
//	%1 = struct $Int (%0)
//
// Until core/ declares the initializer there is nothing to call, so
// this emits the later form. The consequence is where the diff
// points: a function with a literal in it is compared against
// `swiftc -emit-sil`, and one without against `-emit-silgen`.
func (g *gen) literal(e *ast.BasicLit) *vil.Value {
	v, ok := g.info.Values[e]
	if !ok {
		return nil
	}
	t := g.info.Types[e]
	switch v.Kind {
	case analyzer.IntValue:
		raw := g.blk.IntegerLiteral(vil.Object(builtinFor(t)), int64(v.Int))
		return g.blk.Struct(lowerType(t), raw)
	case analyzer.BoolValue:
		n := int64(0)
		if v.Bool {
			n = 1
		}
		raw := g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), n)
		return g.blk.Struct(lowerType(t), raw)
	}
	return nil
}

// builtinFor is the machine type a value of an integer type is one
// of. Int and UInt are the target's word, which is sixty-four bits on
// every target this compiler has a backend for.
func builtinFor(t types.Type) types.Type {
	if t == nil {
		return vil.BuiltinInt64
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return vil.BuiltinInt64
	}
	switch b.Kind() {
	case types.Int8, types.UInt8:
		return vil.BuiltinInt8
	case types.Int16, types.UInt16:
		return vil.BuiltinInt16
	case types.Int32, types.UInt32:
		return vil.BuiltinInt32
	case types.Bool:
		return vil.BuiltinInt1
	}
	return vil.BuiltinInt64
}

// ident lowers a reference to a name.
//
// A let is the value itself, borrowed for the use if something owns
// it — the binding keeps ownership, and the use only needs to see it.
// A var is read through its address, under an access scope, because
// that is where exclusivity is checked.
func (g *gen) ident(e *ast.IdentExpr) *vil.Value {
	sym := g.info.Uses[e.Name]
	if sym == nil {
		return nil
	}
	l := g.locals[sym]
	if l == nil {
		// A function's own name, used as a value rather than called.
		// It is the same pair a closure produces — the declaration
		// referenced, then given the shape of a function value — and
		// it is why `apply(triple, 5)` needs nothing of its own.
		if fs, ok := sym.(*analyzer.FuncSymbol); ok {
			return g.funcValue(fs)
		}
		// Not a local. Inside a method a bare name may be a stored
		// property, which the analyzer resolved to the symbol the type's
		// own scope holds — so it is reached through the receiver, and
		// `x` in a method body means `self.x`.
		if v, ok := g.implicitSelf(e, sym); ok {
			return v
		}
		g.unsupported(e)
		return nil
	}
	if l.addr != nil {
		access := g.blk.BeginAccess(l.addr, "read", "unknown")
		v := g.blk.Load(access, loadQualifier(l.typ))
		g.blk.EndAccess(access)
		return v
	}
	if l.value.Ownership() == vil.Owned {
		return g.borrow(l.value)
	}
	return l.value
}

// implicitSelf reads a stored property of the receiver, for a name in a
// method body that named one.
//
// The property is found by name among the receiver's own, which is what
// the analyzer resolved against: it put the type's stored properties in
// the scope the method body was checked in, so a name that reached here
// and is one of them is that one.
func (g *gen) implicitSelf(e *ast.IdentExpr, sym analyzer.Symbol) (*vil.Value, bool) {
	if g.recv == nil || e.Name == nil {
		return nil, false
	}
	name := g.text(e.Name)
	field, ok := storedField(g.recv, name)
	if !ok {
		return nil, false
	}
	self := g.selfValue()
	if self == nil {
		return nil, false
	}
	t := lowerType(field.Type)
	member := memberName(g.recv, name)
	if isClass(g.recv) {
		addr := g.blk.RefElementAddr(self, member, t)
		access := g.blk.BeginAccess(addr, "read", "dynamic")
		v := g.blk.Load(access, loadQualifier(t))
		g.blk.EndAccess(access)
		return v, true
	}
	return g.blk.StructExtract(self, member, t), true
}

// storedField is the property of a type with this name.
func storedField(t types.Type, name string) (*types.Field, bool) {
	if t == nil {
		return nil, false
	}
	var fields []*types.Field
	switch b := t.Underlying().(type) {
	case *types.Struct:
		fields = b.Fields
	case *types.Class:
		fields = b.Fields
	default:
		return nil, false
	}
	for _, f := range fields {
		if f != nil && f.Name == name {
			return f, true
		}
	}
	return nil, false
}

// borrow opens a borrow that the enclosing scope closes, which is how
// a use sees a value the binding still owns.
func (g *gen) borrow(v *vil.Value) *vil.Value {
	b := g.blk.BeginBorrow(v)
	g.endBorrowLater(b)
	return b
}

// member lowers a property read: borrow the base, take the address of
// the field, read it under an access, and close both.
func (g *gen) member(e *ast.MemberExpr) *vil.Value {
	if e.Name == nil {
		return nil
	}
	// `E.b` names a case rather than reading a property: the base is a
	// type, which has no value to read out of.
	if ec, ok := g.info.Uses[e.Name].(*analyzer.EnumCaseSymbol); ok {
		return g.enumCase(e, ec)
	}
	base := g.expr(e.X)
	if base == nil {
		return nil
	}
	t := lowerType(g.info.Types[e])
	name := memberName(g.info.Types[e.X], e.Name.Text(g.file))

	// A class's property is at an address inside the instance; a
	// struct's is a field of the value.
	if !isClass(g.info.Types[e.X]) {
		return g.blk.StructExtract(base, name, t)
	}
	addr := g.blk.RefElementAddr(base, name, t)
	access := g.blk.BeginAccess(addr, "read", "dynamic")
	v := g.blk.Load(access, loadQualifier(t))
	g.blk.EndAccess(access)
	return v
}

// call lowers a call to a function the checker resolved.
func (g *gen) call(e *ast.CallExpr) *vil.Value {
	// Both refusals report. A call this package returns nothing for
	// and says nothing about is the worst thing it can do: the
	// statement around it drops, the return that wanted the value
	// falls back on whatever a return with no value produces, and a
	// program that should not have compiled runs and is wrong.
	// A call through a dot is a method call, and the receiver is what
	// tells it apart from a call to a free function.
	if mem, ok := e.Fun.(*ast.MemberExpr); ok {
		return g.method(e, mem)
	}
	id, ok := e.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		g.unsupported(e)
		return nil
	}
	// Inside a method, a bare call may name one of the receiver's own
	// methods: `doubled()` there means `self.doubled()`, and its symbol
	// is mangled inside the type rather than beside it.
	if ref, ok := g.implicitMethod(id); ok {
		return g.methodCall(e, ref, func() *vil.Value { return g.selfValue() })
	}
	// A type's name in expression position is a constructor call:
	// `P(x: 1)` names a type rather than a function.
	if tn, ok := g.info.Uses[id.Name].(*analyzer.TypeNameSymbol); ok {
		return g.construct(e, tn)
	}
	// A name bound to a value of function type is called through the
	// value rather than by name: `f(x)` where f is a closure is an
	// apply of what f holds, and there is no symbol to reference.
	if sym := g.info.Uses[id.Name]; sym != nil {
		if _, isFunc := g.info.Types[id].Underlying().(*types.Signature); isFunc {
			if _, isLocal := g.locals[sym]; isLocal {
				return g.applyValue(e, id)
			}
		}
	}

	sym, _ := g.info.Uses[id.Name].(*analyzer.FuncSymbol)
	if sym == nil {
		g.unsupported(e)
		return nil
	}
	callee := g.m.Func(g.symbol(sym)).SetSourceName(sym.Name())
	if callee.IsDeclaration() && callee.Type() != nil {
		g.declare(callee, sym)
	}
	ref := g.blk.FunctionRef(callee)

	var args []*vil.Value
	if e.Args != nil {
		for _, a := range e.Args.Args {
			if v := g.expr(a.X); v != nil {
				args = append(args, v)
			}
		}
	}
	result := lowerType(sym.Signature().Results)
	v := g.blk.Apply(ref, result, args...)
	g.destroyLater(v)
	return v
}

// funcValue is a declared function used as a value.
func (g *gen) funcValue(sym *analyzer.FuncSymbol) *vil.Value {
	callee := g.m.Func(g.symbol(sym)).SetSourceName(sym.Name())
	if callee.IsDeclaration() && callee.Type() != nil {
		g.declare(callee, sym)
	}
	ref := g.blk.FunctionRef(callee)
	return g.blk.ThinToThickFunction(ref, lowerType(sym.Signature()))
}

// applyValue calls a function held in a variable rather than named by
// a declaration — a closure, or a function passed as an argument.
//
// The callee is an operand like any other, which is the whole
// difference from a call by name: there is no function_ref, and what
// is applied is whatever the value holds.
func (g *gen) applyValue(e *ast.CallExpr, id *ast.IdentExpr) *vil.Value {
	sig, _ := g.info.Types[id].Underlying().(*types.Signature)
	if sig == nil {
		g.unsupported(e)
		return nil
	}
	callee := g.expr(id)
	if callee == nil {
		return nil
	}
	var args []*vil.Value
	if e.Args != nil {
		for _, a := range e.Args.Args {
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			args = append(args, v)
		}
	}
	v := g.blk.Apply(callee, lowerType(sig.Results), args...)
	g.destroyLater(v)
	return v
}

// construct lowers making an instance of a type.
//
// A struct with no initializer of its own is made by the memberwise
// one, whose whole body — as `swiftc -emit-sil` prints it — is a
// `struct` instruction over its arguments and a return:
//
//	sil @P.init(x:y:) : $@convention(method) (Int, Int, @thin P.Type) -> P {
//	bb0(%0 : $Int, %1 : $Int, %2 : $@thin P.Type):
//	  %3 = struct $P (%0, %1)
//	  return %3
//	}
//
// So this emits the instruction and not the call. It is the same
// decision literal() documents and takes for the same reason: the
// initializer would have to be declared somewhere before there is
// anything to apply, core/ does not declare it, and a call to a
// function that does not exist is worse than the body of the one that
// would. The consequence is the same too — which swiftc flag the
// output is diffed against.
//
// A class is refused. Making one is an allocation and a reference
// count, which needs alloc_ref lowered and a runtime to call, and
// neither exists yet.
func (g *gen) construct(e *ast.CallExpr, tn *analyzer.TypeNameSymbol) *vil.Value {
	t := g.info.Types[e]
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		if cl, isClass := t.Underlying().(*types.Class); isClass {
			return g.makeClass(e, tn, cl, t)
		}
		g.unsupported(e)
		return nil
	}
	if st.Memberwise() == nil {
		g.errorAt(e, "cannot lower an initializer '"+tn.Name()+
			"' declares itself yet: only the memberwise one is understood")
		return nil
	}

	// The checker matched the arguments to the stored properties and
	// reported anything that did not fit, so what is left is to
	// evaluate them in order. A property left to its default is not
	// lowered yet: the default is an expression on the declaration
	// rather than at the call, and reaching it means going back to
	// the syntax the type was written in.
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	if len(args) != len(st.Fields) {
		g.refuse(e, "a constructor that leaves a property to its default")
		return nil
	}

	values := make([]*vil.Value, 0, len(args))
	for _, a := range args {
		v := g.rvalue(a.X)
		if v == nil {
			return nil
		}
		values = append(values, v)
	}
	made := g.blk.Struct(lowerType(t), values...)
	g.destroyLater(made)
	return made
}

// makeClass makes an instance of a class.
//
// SILGen writes a call to `__allocating_init`, which allocates and
// then runs the initializer; the initializer stores each stored
// property's initial value through `ref_element_addr` and returns
// self. This emits that, inlined, which is the same decision
// construct() takes for a struct and literal() takes for an integer:
// the initializer is not declared anywhere, and a call to a function
// that does not exist is worse than the body of the one that would
// have been called.
//
//	%0 = alloc_ref $Box
//	%1 = ref_element_addr %0, #Box.n
//	%2 = integer_literal $Builtin.Int32, 3
//	%3 = struct $Int32 (%2)
//	store %3 to [trivial] %1
//
// Only the default initializer. A class gets no memberwise one —
// Swift gives that to structs alone, because a class has inheritance
// and initializing it has to account for a superclass — so a class
// with an initializer of its own, or with a property that has no
// initial value, is refused rather than half made.
func (g *gen) makeClass(e *ast.CallExpr, tn *analyzer.TypeNameSymbol, cl *types.Class, t types.Type) *vil.Value {
	if len(cl.Inits) > 0 {
		g.errorAt(e, "cannot lower an initializer '"+tn.Name()+
			"' declares itself yet: only the one a class gets for free is understood")
		return nil
	}
	if e.Args != nil && len(e.Args.Args) > 0 {
		// Swift would refuse this too: without a declared
		// initializer there is nothing for the arguments to go to.
		g.errorAt(e, "'"+tn.Name()+"' has no initializer taking arguments")
		return nil
	}

	// Every stored property has to have somewhere to get its value
	// from, and the only source without a declared initializer is the
	// property's own initial value.
	values := g.classDefaults(tn)
	for _, f := range cl.Fields {
		if f == nil {
			continue
		}
		if _, ok := values[f.Name]; !ok {
			g.errorAt(e, "'"+tn.Name()+"' cannot be made without arguments: '"+
				f.Name+"' has no initial value and there is no initializer to give it one")
			return nil
		}
	}

	obj := g.blk.AllocRef(lowerType(t))
	for _, f := range cl.Fields {
		if f == nil {
			continue
		}
		v := g.rvalue(values[f.Name])
		if v == nil {
			return nil
		}
		ft := lowerType(f.Type)
		addr := g.blk.RefElementAddr(obj, memberName(t, f.Name), ft)
		g.blk.Store(v, addr, storeQualifier(ft))
	}
	g.destroyLater(obj)
	return obj
}

// classDefaults is the initial value each stored property was
// declared with, by name.
//
// It goes back to the syntax because that is where a default lives:
// types/ holds no expressions on purpose, so the type can say that a
// property has a value but not what it is.
func (g *gen) classDefaults(tn *analyzer.TypeNameSymbol) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	body := classBody(tn.Decl())
	if body == nil {
		return out
	}
	for _, mem := range body.Members {
		vd, ok := mem.(*ast.VarDecl)
		if !ok {
			continue
		}
		for _, b := range vd.Bindings {
			if b.Value == nil {
				continue
			}
			if name := g.bindingName(b.Pat); name != "" {
				out[name] = b.Value
			}
		}
	}
	return out
}

// classBody is the member block of whichever declaration declares a
// class.
func classBody(d ast.Decl) *ast.MemberBlock {
	switch n := d.(type) {
	case *ast.ClassDecl:
		return n.Body
	case *ast.ActorDecl:
		return n.Body
	}
	return nil
}

// bindingName is the name a pattern binds, where it binds exactly
// one. A destructuring pattern is not a stored property.
func (g *gen) bindingName(p ast.Pattern) string {
	if tp, ok := p.(*ast.TypedPattern); ok {
		p = tp.Pat
	}
	if id, ok := p.(*ast.IdentPattern); ok {
		return g.text(id.Name)
	}
	return ""
}

// enumCase builds one case of an enum, which SILGen writes as
//
//	%1 = enum $E, #E.b!enumelt
//
// A case that carries associated values is refused: the payload has to
// be put in, and where it goes is a layout this compiler does not
// compute yet.
func (g *gen) enumCase(e ast.Expr, ec *analyzer.EnumCaseSymbol) *vil.Value {
	if ec.AssociatedType() != nil {
		g.refuse(e, "an enum case that carries a value")
		return nil
	}
	t := g.info.Types[e]
	if t == nil {
		t = ec.Type()
	}
	return g.blk.Enum(lowerType(t), memberName(ec.Type(), ec.Name()), nil)
}

// method lowers a call through a dot.
//
// It is an ordinary call with the receiver passed as an argument, which
// is what `swiftc -emit-sil` shows for both a struct's method and a
// final class's:
//
//	sil @P.add : $@convention(method) (Int, Int, P) -> Int
//	%6 = function_ref @P.add
//	%7 = apply %6(%3, %5, %0)
//
// Self goes last. That is the method convention's shape and not an
// arbitrary choice: it is what puts the receiver in the register a
// method finds it in, and reversing it would pass the first argument as
// the receiver.
//
// Static where that is the only answer, dynamic where it is not.
//
// A method on a class that neither has a superclass nor is one has a
// single implementation, so a function_ref names the only thing there
// is to name. Where inheritance is involved, which body runs is a fact
// about the object, and the call goes through the table the instance
// carries -- see dynamicCall and vil/gen/vtable.go.
//
// This comment once said inheritance was not modelled, and treated
// static dispatch as safe on that ground. The checker learned
// inheritance -- it resolves a method through the superclass chain and
// converts a subclass to its base -- and this did not notice, so a
// call through a base-typed reference ran the base's body and returned
// the wrong number with nothing said. An assumption about another
// package is only as good as the day it was written down.
func (g *gen) method(e *ast.CallExpr, mem *ast.MemberExpr) *vil.Value {
	ref := g.info.Methods[mem]
	if ref == nil || ref.Method == nil || ref.Method.Sig == nil {
		g.unsupported(e)
		return nil
	}
	if g.info.Types[mem.X] == nil {
		g.unsupported(e)
		return nil
	}
	return g.methodCall(e, ref, func() *vil.Value {
		// The receiver is borrowed rather than copied: self is
		// @guaranteed, which says the caller keeps it alive across the
		// call and the callee does not consume it. rvalue would hand
		// over a copy nothing afterwards destroys.
		return g.expr(mem.X)
	})
}

// methodCall emits the call itself, over a receiver the caller supplies.
func (g *gen) methodCall(e *ast.CallExpr, ref *analyzer.MethodRef, receiver func() *vil.Value) *vil.Value {
	// Which body a method call reaches is a fact about the object when
	// the class takes part in inheritance, so it is asked of the
	// object: class_method reads the slot out of the table the
	// instance carries. A function_ref here would name the static
	// type's body, which is what this used to do -- it called the
	// superclass's and returned the wrong number, with nothing said.
	if cl, ok := receiverClass(ref.Recv); ok && g.poly[cl] {
		return g.dynamicCall(e, ref, cl, receiver)
	}
	callee := g.m.Func(g.methodSymbol(ref)).SetSourceName(ref.Method.Name)
	if callee.IsDeclaration() && callee.Type() != nil {
		g.declareMethod(callee, ref)
	}
	fnRef := g.blk.FunctionRef(callee)

	// The arguments the program wrote, then the receiver.
	var args []*vil.Value
	if e.Args != nil {
		for _, a := range e.Args.Args {
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			args = append(args, v)
		}
	}
	self := receiver()
	if self == nil {
		g.unsupported(e)
		return nil
	}
	args = append(args, self)

	result := lowerType(ref.Method.Sig.Results)
	v := g.blk.Apply(fnRef, result, args...)
	g.destroyLater(v)
	return v
}

// dynamicCall dispatches through the receiver's table.
//
// The slot is named for the class that introduced the method rather
// than the one the lookup found it in: an override does not get a slot
// of its own, it replaces the one the base declared, and B's table
// says #A.get. A call site that named #B.get would be asking for a row
// that is not there.
func (g *gen) dynamicCall(e *ast.CallExpr, ref *analyzer.MethodRef, cl *types.Class,
	receiver func() *vil.Value) *vil.Value {

	var args []*vil.Value
	if e.Args != nil {
		for _, a := range e.Args.Args {
			v := g.rvalue(a.X)
			if v == nil {
				return nil
			}
			args = append(args, v)
		}
	}
	self := receiver()
	if self == nil {
		g.unsupported(e)
		return nil
	}

	intro := introducer(cl, ref.Method)
	member := intro.Name + "." + ref.Method.Name
	method := g.blk.ClassMethod(self, member, methodType(ref, intro))

	args = append(args, self)
	result := lowerType(ref.Method.Sig.Results)
	v := g.blk.Apply(method, result, args...)
	g.destroyLater(v)
	return v
}

// methodType is the function type a class_method yields: the method's
// own, with the receiver last and the introducing class as its type,
// because that is the signature every implementation of the slot has
// to have.
func methodType(ref *analyzer.MethodRef, intro *types.Class) vil.Type {
	sig := ref.Method.Sig
	ft := &vil.FuncType{Convention: vil.Method}
	for _, p := range sig.Params {
		t := lowerType(p.Type)
		ft.Params = append(ft.Params, vil.Param{Type: t, Convention: paramConvention(p, t)})
	}
	self := lowerType(intro)
	ft.Params = append(ft.Params, vil.Param{Type: self, Convention: selfConvention(self)})
	if sig.Results != nil && !isVoid(sig.Results) {
		t := lowerType(sig.Results)
		ft.Results = append(ft.Results, vil.Result{Type: t, Convention: resultConvention(t)})
	}
	return vil.Object(ft)
}

// introducer is the base-most class declaring this method, which is
// the class the slot is named for.
func introducer(cl *types.Class, m *types.Method) *types.Class {
	if m == nil || m.Sig == nil {
		return cl
	}
	key := m.Name + m.Sig.String()
	for _, c := range classChain(cl) {
		for _, own := range c.Methods {
			if own != nil && own.Sig != nil && own.Name+own.Sig.String() == key {
				return c
			}
		}
	}
	return cl
}

// receiverClass is the class a method was found in, if it was found in
// one. A struct or an enum has no inheritance and no question to ask.
func receiverClass(t types.Type) (*types.Class, bool) {
	if t == nil {
		return nil, false
	}
	cl, ok := t.Underlying().(*types.Class)
	return cl, ok
}

// implicitMethod is the receiver's method a bare name refers to, for a
// call written inside one of its own methods.
func (g *gen) implicitMethod(id *ast.IdentExpr) (*analyzer.MethodRef, bool) {
	if g.recv == nil || id.Name == nil {
		return nil, false
	}
	name := g.text(id.Name)
	var methods []*types.Method
	switch b := g.recv.Underlying().(type) {
	case *types.Struct:
		methods = b.Methods
	case *types.Class:
		methods = b.Methods
	case *types.Enum:
		methods = b.Methods
	default:
		return nil, false
	}
	for _, m := range methods {
		if m != nil && m.Name == name {
			return &analyzer.MethodRef{Recv: g.recv, Method: m}, true
		}
	}
	return nil, false
}

// methodSymbol is the name a method is given: the mangling of a
// declaration nested inside the type that declares it.
//
// The type is what makes it a method's symbol rather than a free
// function's, which is why mangle.Decl carries a context at all. Two
// types may each declare `doubled`, and they are two symbols.
func (g *gen) methodSymbol(ref *analyzer.MethodRef) string {
	d := mangle.Decl{
		Module:    g.module,
		Name:      ref.Method.Name,
		Signature: ref.Method.Sig,
	}
	if nom, ok := nominalOf(ref.Recv); ok {
		d.Context = []mangle.Nominal{nom}
	}
	name, err := mangle.Function(d)
	if err != nil {
		g.errorAt(nil, "cannot name '"+ref.Method.Name+"': "+err.Error())
		return ref.Method.Name
	}
	return name
}

// nominalOf is the mangler's name for a type: what it is called and
// which of the three kinds it is.
func nominalOf(t types.Type) (mangle.Nominal, bool) {
	if t == nil {
		return mangle.Nominal{}, false
	}
	name := typeName(t)
	if name == "" {
		return mangle.Nominal{}, false
	}
	switch t.Underlying().(type) {
	case *types.Struct:
		return mangle.Nominal{Name: name, Kind: mangle.Struct}, true
	case *types.Class:
		return mangle.Nominal{Name: name, Kind: mangle.Class}, true
	case *types.Enum:
		return mangle.Nominal{Name: name, Kind: mangle.Enum}, true
	}
	return mangle.Nominal{}, false
}

// declareMethod gives a method's callee its lowered type: the
// parameters the program wrote, then the receiver.
func (g *gen) declareMethod(f *vil.Func, ref *analyzer.MethodRef) {
	sig := ref.Method.Sig
	for _, p := range sig.Params {
		t := lowerType(p.Type)
		f.Type().Params = append(f.Type().Params,
			vil.Param{Type: t, Convention: paramConvention(p, t)})
	}
	self := lowerType(ref.Recv)
	f.Type().Params = append(f.Type().Params,
		vil.Param{Type: self, Convention: selfConvention(self)})
	f.Type().Convention = vil.Method
	if sig.Results != nil && !isVoid(sig.Results) {
		t := lowerType(sig.Results)
		f.SetResult(t, resultConvention(t))
	}
}

// selfConvention is how a receiver crosses the call. A struct is a value
// and owns nothing the caller has to keep alive; a class is a reference
// the caller holds for the duration, which is @guaranteed and is what
// SILGen writes.
func selfConvention(t vil.Type) vil.ParamConvention {
	if t.Trivial() {
		return vil.ParamUnowned
	}
	return vil.ParamGuaranteed
}

// declare gives a callee its lowered type, so that a call to a
// function not yet lowered still names the right thing.
func (g *gen) declare(f *vil.Func, sym *analyzer.FuncSymbol) {
	sig := sym.Signature()
	for _, p := range sig.Params {
		t := lowerType(p.Type)
		f.Type().Params = append(f.Type().Params,
			vil.Param{Type: t, Convention: paramConvention(p, t)})
	}
	if sig.Results != nil && !isVoid(sig.Results) {
		t := lowerType(sig.Results)
		f.SetResult(t, resultConvention(t))
	}
}

// binary lowers an operator.
//
// An operator is a function and the checker resolved it to one, in
// core. What core declares has no body, because its body is a machine
// instruction — so the call is not emitted; the instruction is.
//
// The shape is Swift's, and it is three steps. Reach through the
// struct to the machine value; do the arithmetic, which for a checked
// operator returns the result and an overflow bit; wrap what comes
// back in the type that holds it.
//
//	%4 = struct_extract %0, #Int._value
//	%5 = struct_extract %1, #Int._value
//	%6 = integer_literal $Builtin.Int1, -1
//	%7 = builtin "sadd_with_overflow_Int64"(%4, %5, %6) : $(Builtin.Int64, Builtin.Int1)
//	%8 = tuple_extract %7, 0
//	%9 = tuple_extract %7, 1
//	cond_fail %9, "arithmetic overflow"
//	%10 = struct $Int (%8)
func (g *gen) binary(e *ast.BinaryExpr) *vil.Value {
	sym, _ := g.info.Operators[e].(*analyzer.FuncSymbol)
	if sym == nil {
		g.expr(e.X)
		g.expr(e.Y)
		g.unsupported(e)
		return nil
	}
	op := sym.Name()

	// && and || do not evaluate their right operand unless they have
	// to, which is a branch rather than an instruction.
	if op == "&&" || op == "||" {
		return g.shortCircuit(e, op == "&&")
	}

	operand := g.info.Types[e.X]
	bi, ok := core.Lower(op, operand)
	if !ok {
		g.expr(e.X)
		g.expr(e.Y)
		g.unsupported(e)
		return nil
	}
	lhs, rhs := g.expr(e.X), g.expr(e.Y)
	if lhs == nil || rhs == nil {
		return nil
	}

	a, b := g.machine(lhs, operand), g.machine(rhs, operand)
	result := lowerType(sym.Signature().Results)

	if !bi.Overflows {
		raw := g.blk.Builtin(bi.Name, vil.Object(builtinNamed(bi.Result)), a, b)
		return g.blk.Struct(result, raw)
	}

	// A checked operator asks for the trap it wants: -1 is all bits
	// set, which is how Swift says "report the overflow".
	want := g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), -1)
	pair := vil.Object(&types.Tuple{Elements: []*types.TupleElement{
		{Type: builtinNamed(bi.Result)},
		{Type: vil.BuiltinInt1},
	}})
	both := g.blk.Builtin(bi.Name, pair, a, b, want)
	value := g.blk.TupleExtract(both, 0, vil.Object(builtinNamed(bi.Result)))
	flag := g.blk.TupleExtract(both, 1, vil.Object(vil.BuiltinInt1))
	g.blk.CondFail(flag, "arithmetic overflow")
	return g.blk.Struct(result, value)
}

// conditional lowers `a ? b : c`.
//
// The same shape as && and || and for the same reason: two paths
// produce the answer in different blocks, so the join takes it as a
// parameter. What differs is that both sides are expressions the
// program wrote, where a short circuit has a constant on one side.
//
// The parser leaves a ternary as an operator inside a sequence, with
// only its middle operand attached; the analyzer's fold is what pairs
// it with the condition and the else and produces the
// *ast.ConditionalExpr this reads.
func (g *gen) conditional(e *ast.ConditionalExpr) *vil.Value {
	cond := g.expr(e.Cond)
	if cond == nil {
		return nil
	}
	bit := g.machine(cond, g.info.Types[e.Cond])
	result := lowerType(g.info.Types[e])

	thenBlk := g.fn.Block()
	elseBlk := g.fn.Block()
	join := g.fn.Block()
	answer := join.Arg(result, joinOwnership(result))

	g.blk.CondBr(bit, thenBlk, nil, elseBlk, nil)

	g.blk = thenBlk
	yes := g.rvalue(e.Then)
	if yes == nil {
		return nil
	}
	g.blk.Br(join, yes)

	g.blk = elseBlk
	no := g.rvalue(e.Else)
	if no == nil {
		return nil
	}
	g.blk.Br(join, no)

	g.blk = join
	// Whichever branch ran produced something the join now owns, and
	// exactly one of them ran — so it is destroyed once, where the
	// scope holding it ends.
	g.destroyLater(answer)
	return answer
}

// joinOwnership is what a block argument carrying a value from two
// paths owns. Each path handed over what it made, so the join owns it
// and is responsible for it.
func joinOwnership(t vil.Type) vil.Ownership {
	if t.Trivial() {
		return vil.None
	}
	return vil.Owned
}

// shortCircuit lowers && and ||, which are branches rather than
// instructions: the right operand is evaluated only when the left one
// did not already decide the answer.
//
// Canonical SIL's shape, and the reason the answer arrives as a block
// argument rather than a value:
//
//	  <left>
//	  cond_br %bit, bb1, bb2      // for ||, the destinations swap
//	bb1:                          // undecided: the answer is the right operand
//	  <right>
//	  br bb3(%right)
//	bb2:                          // decided: false for &&, true for ||
//	  %c = integer_literal $Builtin.Int1, 0
//	  %b = struct $Bool (%c)
//	  br bb3(%b)
//	bb3(%r : $Bool):
//
// The two paths produce the answer in different blocks, and a value
// is usable only where it dominates its readers — so the join takes
// it as a parameter, which is what SILGen does and what SSA requires.
//
// A borrow opened by the right operand would want ending on the path
// that evaluated it and not on the path that did not; nothing
// arranges that here. It is unreachable while the operands are Bools,
// which own nothing. An operand that owns something needs the scope
// handling before it needs anything else.
func (g *gen) shortCircuit(e *ast.BinaryExpr, isAnd bool) *vil.Value {
	lhs := g.expr(e.X)
	if lhs == nil {
		return nil
	}
	bit := g.machine(lhs, g.info.Types[e.X])
	result := lowerType(g.info.Types[e])

	rhsBlk := g.fn.Block()
	decided := g.fn.Block()
	join := g.fn.Block()
	// The join's parameter exists before anything branches to it: a
	// branch carries the values the block takes, so the block has to
	// have said what it takes first.
	answer := join.Arg(result, joinOwnership(result))

	// `a && b` goes right when a was true; `a || b` goes right when a
	// was false. That is the whole difference between them.
	if isAnd {
		g.blk.CondBr(bit, rhsBlk, nil, decided, nil)
	} else {
		g.blk.CondBr(bit, decided, nil, rhsBlk, nil)
	}

	g.blk = rhsBlk
	rhs := g.expr(e.Y)
	if rhs == nil {
		return nil
	}
	g.blk.Br(join, rhs)

	// The operand that was not evaluated decided the answer: false for
	// &&, true for ||.
	g.blk = decided
	n := int64(0)
	if !isAnd {
		n = 1
	}
	raw := g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), n)
	g.blk.Br(join, g.blk.Struct(result, raw))

	g.blk = join
	return answer
}

// machine reaches through a primitive's struct to the word inside it,
// which is what the builtin operates on.
func (g *gen) machine(v *vil.Value, t types.Type) *vil.Value {
	field, name, ok := core.Layout(t)
	if !ok {
		return v
	}
	return g.blk.StructExtract(v, typeName(t)+"."+field,
		vil.Object(builtinNamed(name)))
}

// builtinNamed is the builtin type core named.
func builtinNamed(name string) types.Type {
	switch name {
	case "Int1":
		return vil.BuiltinInt1
	case "Int8":
		return vil.BuiltinInt8
	case "Int16":
		return vil.BuiltinInt16
	case "Int32":
		return vil.BuiltinInt32
	case "FPIEEE32":
		return vil.BuiltinFPIEEE32
	case "FPIEEE64":
		return vil.BuiltinFPIEEE64
	}
	return vil.BuiltinInt64
}

// selfValue is the receiver, which is the last parameter of a method.
func (g *gen) selfValue() *vil.Value {
	if g.fn == nil {
		return nil
	}
	args := g.fn.Entry().Args()
	if len(args) == 0 {
		return nil
	}
	return args[len(args)-1]
}

// loadQualifier says what a load does to ownership.
func loadQualifier(t vil.Type) string {
	if t.Trivial() {
		return "trivial"
	}
	return "copy"
}

// memberName is how a member is written in the text form: the type's
// name, then the member's.
func memberName(base types.Type, name string) string {
	if base == nil {
		return name
	}
	return typeName(base) + "." + name
}

func typeName(t types.Type) string {
	if named, ok := t.(*types.Named); ok {
		return named.Name
	}
	return t.String()
}

func isClass(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Class)
	return ok
}
