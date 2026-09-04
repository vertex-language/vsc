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
	id, ok := e.Fun.(*ast.IdentExpr)
	if !ok || id.Name == nil {
		return nil
	}
	sym, _ := g.info.Uses[id.Name].(*analyzer.FuncSymbol)
	if sym == nil {
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
		return nil
	}
	op := sym.Name()

	// && and || do not evaluate their right operand unless they have
	// to, which is a branch rather than an instruction. They wait on
	// the lowering that gives them one.
	if op == "&&" || op == "||" {
		g.expr(e.X)
		g.expr(e.Y)
		return nil
	}

	operand := g.info.Types[e.X]
	bi, ok := core.Lower(op, operand)
	if !ok {
		g.expr(e.X)
		g.expr(e.Y)
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
