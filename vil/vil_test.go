package vil

import (
	"testing"

	"github.com/vertex-language/vsc/types"
)

func classType(name string) types.Type {
	return types.NewNamed(name, "", &types.Class{Name: name})
}

// TestTrivial covers the question every ownership decision starts
// from: does a value of this type own anything? An Int does not, a
// class reference does, and an aggregate does exactly when one of its
// members does.
func TestTrivial(t *testing.T) {
	box := classType("Box")
	cases := []struct {
		name string
		typ  Type
		want bool
	}{
		{"Int", Object(types.Typ[types.Int]), true},
		{"Bool", Object(types.Typ[types.Bool]), true},
		{"Double", Object(types.Typ[types.Double]), true},
		{"String", Object(types.Typ[types.String]), false},
		{"Box", Object(box), false},
		{"Builtin.Int64", Object(BuiltinInt64), true},
		{"Builtin.NativeObject", Object(BuiltinNativeObj), false},
		{"Int.Type", Object(&types.Metatype{Instance: types.Typ[types.Int]}), true},
		{"struct of Int", Object(&types.Struct{Name: "P", Fields: []*types.Field{
			{Name: "a", Type: types.Typ[types.Int]}}}), true},
		{"struct holding a class", Object(&types.Struct{Name: "Q", Fields: []*types.Field{
			{Name: "a", Type: types.Typ[types.Int]}, {Name: "b", Type: box}}}), false},
		{"tuple of Int", Object(&types.Tuple{Elements: []*types.TupleElement{
			{Type: types.Typ[types.Int]}}}), true},
	}
	for _, c := range cases {
		if got := c.typ.Trivial(); got != c.want {
			t.Errorf("%s.Trivial() = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestTypeText holds the two spellings apart: a value and the address
// of one.
func TestTypeText(t *testing.T) {
	i := types.Typ[types.Int]
	if got := Object(i).String(); got != "$Int" {
		t.Errorf("object type printed %q", got)
	}
	if got := Address(i).String(); got != "$*Int" {
		t.Errorf("address type printed %q", got)
	}
	if !Address(i).IsAddress() || Object(i).IsAddress() {
		t.Errorf("address bit is wrong")
	}
	if !Object(i).Equal(Address(i).Object()) {
		t.Errorf("an address's object type should be the object type")
	}
}

// TestResultOwnership is the table the whole IR rests on: what each
// instruction's result owns. Getting one of these wrong is what makes
// rule one unenforceable.
func TestResultOwnership(t *testing.T) {
	box := classType("Box")
	boxT, intT := Object(box), Object(types.Typ[types.Int])

	m := NewModule("t", StageRaw)
	f := m.Func("f").SetAttr("ossa")
	owned := f.Param(boxT, ParamOwned)
	guaranteed := f.Param(boxT, ParamGuaranteed)
	plain := f.Param(intT, ParamUnowned)
	bb := f.Entry()

	if owned.Ownership() != Owned {
		t.Errorf("an @owned parameter is %v", owned.Ownership())
	}
	if guaranteed.Ownership() != Guaranteed {
		t.Errorf("a @guaranteed parameter is %v", guaranteed.Ownership())
	}
	if plain.Ownership() != None {
		t.Errorf("a trivial parameter is %v, want none", plain.Ownership())
	}

	if got := bb.CopyValue(guaranteed).Ownership(); got != Owned {
		t.Errorf("copy_value is %v, want @owned", got)
	}
	if got := bb.BeginBorrow(owned).Ownership(); got != Guaranteed {
		t.Errorf("begin_borrow is %v, want @guaranteed", got)
	}
	if got := bb.AllocRef(boxT).Ownership(); got != Owned {
		t.Errorf("alloc_ref is %v, want @owned", got)
	}
	// An address is not a value: nothing owns it.
	if got := bb.AllocStack(boxT).Ownership(); got != None {
		t.Errorf("alloc_stack is %v, want none", got)
	}
	if got := bb.RefElementAddr(guaranteed, "Box.n", intT).Ownership(); got != None {
		t.Errorf("ref_element_addr is %v, want none", got)
	}
	// A load takes its ownership from what it was told to do.
	addr := bb.AllocStack(boxT)
	if got := bb.Load(addr, "copy").Ownership(); got != Owned {
		t.Errorf("load [copy] is %v, want @owned", got)
	}
	if got := bb.Load(bb.AllocStack(intT), "trivial").Ownership(); got != None {
		t.Errorf("load [trivial] is %v, want none", got)
	}
}

// TestUsesAndConsumers checks the def-use graph the ownership rules
// are read off.
func TestUsesAndConsumers(t *testing.T) {
	box := classType("Box")
	m := NewModule("t", StageRaw)
	f := m.Func("f").SetAttr("ossa")
	b := f.Param(Object(box), ParamOwned)
	f.SetResult(Object(box), ResultOwned)

	bb := f.Entry()
	borrow := bb.BeginBorrow(b)
	bb.EndBorrow(borrow)
	bb.Return(b)

	if got := len(b.Uses()); got != 2 {
		t.Errorf("the parameter has %d uses, want 2", got)
	}
	// begin_borrow does not consume; return does.
	cs := b.Consumers()
	if len(cs) != 1 || cs[0].Op() != Return {
		t.Errorf("consumers are %v, want one return", cs)
	}
}

// TestBlocks covers labels, terminators and the predecessor list,
// which is computed rather than stored so that it cannot be wrong.
func TestBlocks(t *testing.T) {
	intT := Object(types.Typ[types.Int])
	m := NewModule("t", StageRaw)
	f := m.Func("f").SetAttr("ossa")
	f.SetResult(intT, ResultUnowned)

	entry := f.Entry()
	yes, no, join := f.Block(), f.Block(), f.Block()
	cond := entry.IntegerLiteral(Object(BuiltinInt1), 1)
	entry.CondBr(cond, yes, nil, no, nil)
	yes.Br(join, yes.IntegerLiteral(intT, 1))
	no.Br(join, no.IntegerLiteral(intT, 2))
	out := join.Arg(intT, None)
	join.Return(out)

	if entry.Label() != "bb0" || join.Label() != "bb3" {
		t.Errorf("labels are %q and %q", entry.Label(), join.Label())
	}
	if entry.Term() == nil || entry.Term().Op() != CondBr {
		t.Errorf("the entry block's terminator is %v", entry.Term())
	}
	if got := len(join.Preds()); got != 2 {
		t.Errorf("the join block has %d predecessors, want 2", got)
	}
	if got := len(entry.Preds()); got != 0 {
		t.Errorf("the entry block has %d predecessors, want none", got)
	}
	if got := entry.Term().Successors(); len(got) != 2 {
		t.Errorf("cond_br has %d successors, want 2", len(got))
	}
}

// TestFuncType prints the lowered function types SIL writes.
func TestFuncType(t *testing.T) {
	box := classType("Box")
	err := classType("Error")
	cases := []struct {
		name string
		typ  *FuncType
		want string
	}{
		{"thin, no arguments", &FuncType{Convention: Thin},
			"@convention(thin) () -> ()"},
		{"borrowed in, plain out", &FuncType{Convention: Thin,
			Params:  []Param{{Object(box), ParamGuaranteed}},
			Results: []Result{{Object(types.Typ[types.Int]), ResultUnowned}}},
			"@convention(thin) (@guaranteed Box) -> Int"},
		{"owned in and out", &FuncType{Convention: Thin,
			Params:  []Param{{Object(box), ParamOwned}},
			Results: []Result{{Object(box), ResultOwned}}},
			"@convention(thin) (@owned Box) -> @owned Box"},
		{"inout", &FuncType{Convention: Thin,
			Params: []Param{{Address(box), ParamInout}}},
			"@convention(thin) (@inout *Box) -> ()"},
		{"method", &FuncType{Convention: Method,
			Params:  []Param{{Object(types.Typ[types.Int]), ParamUnowned}},
			Results: []Result{{Object(types.Typ[types.Int]), ResultUnowned}}},
			"@convention(method) (Int) -> Int"},
		{"throwing", &FuncType{Convention: Thin,
			Results:   []Result{{Object(types.Typ[types.Int]), ResultUnowned}},
			ErrorType: Object(err)},
			"@convention(thin) () -> (Int, @error Error)"},
	}
	for _, c := range cases {
		if got := c.typ.String(); got != c.want {
			t.Errorf("%s printed\n  %s\nwant\n  %s", c.name, got, c.want)
		}
	}
}

// TestOpClassification covers what the verifier will ask of an
// opcode.
func TestOpClassification(t *testing.T) {
	for _, op := range []Op{Br, CondBr, SwitchEnum, Return, Throw, Unreachable} {
		if !op.IsTerminator() {
			t.Errorf("%s should end a block", op)
		}
	}
	for _, op := range []Op{CopyValue, Load, Apply, StructExtract} {
		if op.IsTerminator() {
			t.Errorf("%s should not end a block", op)
		}
	}
	if !DestroyValue.Consumes(0) || !Return.Consumes(0) {
		t.Errorf("destroy_value and return consume their operand")
	}
	if BeginBorrow.Consumes(0) || EndBorrow.Consumes(0) {
		t.Errorf("borrowing does not consume")
	}
	if !Store.Consumes(0) || Store.Consumes(1) {
		t.Errorf("store consumes the value, not the address")
	}
}
