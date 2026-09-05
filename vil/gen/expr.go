package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
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
	base := g.expr(e.X)
	if base == nil || e.Name == nil {
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
	id, ok := e.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		g.unsupported(e)
		return nil
	}
	// A type's name in expression position is a constructor call:
	// `P(x: 1)` names a type rather than a function.
	if tn, ok := g.info.Uses[id.Name].(*analyzer.TypeNameSymbol); ok {
		return g.construct(e, tn)
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
