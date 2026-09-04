package lower

import (
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// inst translates one VIL instruction.
//
// Three kinds appear here. Some are arithmetic and become arithmetic.
// Some only change what a value is called -- a struct wrapping one
// field, the struct_extract that takes it back out, an access scope
// around an address -- and become nothing at all, the destination
// simply naming the same register. The rest are refused by name.
func (c *fn) inst(in *vil.Inst) error {
	switch in.Op() {

	// --- the ones that cost nothing ---

	case vil.DebugValue:
		if v, ok := c.value(in.Args()[0]); ok && in.Aux().Name != "" && v.Def() != nil {
			if v.Def().Name() == "" {
				v.Def().SetName(in.Aux().Name)
			}
		}
		return nil

	case vil.Struct:
		// A struct of one field is that field once the names are gone.
		// Int is a Builtin.Int64 and Bool is a Builtin.Int1, and the
		// register does not know the difference.
		if len(in.Args()) != 1 {
			return c.fail(ErrUnsupported, in.Op(), "a struct of more than one field")
		}
		c.forward(in.Result(), in.Args()[0])
		return nil

	case vil.StructExtract:
		c.forward(in.Result(), in.Args()[0])
		return nil

	case vil.BeginAccess:
		// Where exclusivity is enforced is a question this package is
		// downstream of: by now the checking either happened or was
		// found unnecessary, and the address is the address.
		c.forward(in.Result(), in.Args()[0])
		return nil

	case vil.EndAccess, vil.DeallocStack, vil.EndLifetime, vil.ExtendLifetime:
		return nil

	case vil.Tuple:
		// The only tuple this package holds in a register is the one
		// with nothing in it, which is no register at all.
		if len(in.Args()) != 0 {
			return c.fail(ErrUnsupported, in.Op(), "a tuple of more than nothing")
		}
		return nil

	case vil.RefElementAddr:
		return c.refElementAddr(in)

	case vil.FunctionRef:
		callee, ok := c.l.callee[in.Aux().Name]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "no such function: "+in.Aux().Name)
		}
		c.refs[in.Result()] = callee
		return nil

	// --- literals ---

	case vil.IntegerLiteral:
		// Swift passes a with_overflow builtin a third operand saying
		// whether the overflow should be reported. VIR asks the
		// question with a separate instruction and has nothing to do
		// with the answer, so a literal that exists only to be that
		// operand is never materialized.
		if onlyOverflowFlag(in.Result()) {
			return nil
		}
		return c.integerLiteral(in)

	// --- arithmetic ---

	case vil.BuiltinCall:
		return c.builtinCall(in)

	case vil.TupleExtract:
		parts, ok := c.multi[in.Args()[0]]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "a tuple that is not a builtin's results")
		}
		i := int(in.Aux().Int)
		if i < 0 || i >= len(parts) {
			return c.fail(ErrUnsupported, in.Op(), "element out of range")
		}
		c.def(in.Result(), parts[i])
		return nil

	case vil.CondFail:
		return c.condFail(in)

	// --- memory ---

	case vil.AllocStack:
		return c.allocStack(in)
	case vil.Load:
		return c.load(in)
	case vil.Store:
		return c.store(in)

	// --- lifetime, now that it is calls ---

	case vil.StrongRetain:
		return c.refCount(in, &c.l.retain, "vertex_retain")
	case vil.StrongRelease:
		return c.refCount(in, &c.l.release, "vertex_release")

	// --- calls and control flow ---

	case vil.Apply:
		return c.apply(in)
	case vil.Br:
		return c.br(in)
	case vil.CondBr:
		return c.condBr(in)
	case vil.Return:
		return c.ret(in)
	case vil.Unreachable:
		c.b.Trap()
		return nil
	}
	return c.fail(ErrUnsupported, in.Op(), "")
}

// operand is the register an instruction reads, and an error when
// there is not one -- which means either a value this package holds in
// memory, or a function reference used somewhere other than a call.
func (c *fn) operand(in *vil.Inst, v *vil.Value) (ir.Value, error) {
	got, ok := c.value(v)
	if !ok {
		if _, isRef := c.refs[v]; isRef {
			return nil, c.fail(ErrUnsupported, in.Op(), "a function reference used other than as a callee")
		}
		return nil, c.fail(ErrUnsupported, in.Op(), "an operand held in memory")
	}
	return got, nil
}

func (c *fn) operands(in *vil.Inst) ([]ir.Value, error) {
	return c.operandsOf(in, in.Args())
}

func (c *fn) operandsOf(in *vil.Inst, vs []*vil.Value) ([]ir.Value, error) {
	out := make([]ir.Value, len(vs))
	for i, a := range vs {
		got, err := c.operand(in, a)
		if err != nil {
			return nil, err
		}
		out[i] = got
	}
	return out, nil
}

// onlyOverflowFlag reports whether every use of a value is as the
// ignored third operand of an overflow-reporting builtin.
func onlyOverflowFlag(v *vil.Value) bool {
	if v == nil || len(v.Uses()) == 0 {
		return false
	}
	for _, u := range v.Uses() {
		if u.Op() != vil.BuiltinCall || len(u.Args()) < 3 || u.Args()[2] != v {
			return false
		}
		if verb, _, ok := splitBuiltin(u.Aux().Name); !ok || !strings.HasSuffix(verb, "_with_overflow") {
			return false
		}
	}
	return true
}

func (c *fn) integerLiteral(in *vil.Inst) error {
	res := in.Result()
	r, ok := machine(res.Type())
	if !ok {
		return c.fail(ErrType, in.Op(), res.Type().String())
	}
	n := in.Aux().Int
	switch r.reg {
	case ir.TypeI1:
		// SIL writes a true Builtin.Int1 as -1: it is one bit, all of
		// them set.
		c.def(res, c.b.I1.Const(n != 0))
	case ir.TypeI32:
		c.def(res, c.b.I32.Const(n))
	case ir.TypeI64:
		c.def(res, c.b.I64.Const(n))
	default:
		return c.fail(ErrType, in.Op(), r.reg.String())
	}
	return nil
}

func (c *fn) builtinCall(in *vil.Inst) error {
	operands := in.Args()
	if verb, _, ok := splitBuiltin(in.Aux().Name); ok &&
		strings.HasSuffix(verb, "_with_overflow") && len(operands) > 2 {
		operands = operands[:2]
	}
	args, err := c.operandsOf(in, operands)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return c.fail(ErrBuiltin, in.Op(), in.Aux().Name+": no operands")
	}
	got, err := c.builtin(in.Aux().Name, args)
	if err != nil {
		return err
	}
	res := in.Result()
	switch len(got) {
	case 1:
		c.def(res, got[0])
	default:
		c.multi[res] = got
	}
	return nil
}

// condFail is a trap. Swift's overflow checks and preconditions have
// nowhere to unwind to and no error to throw, so the failing edge goes
// to a block that traps and the block splits around it.
func (c *fn) condFail(in *vil.Inst) error {
	cond, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	bit, ok := cond.(ir.I1)
	if !ok {
		return c.fail(ErrType, in.Op(), "the condition is not an i1")
	}
	c.conts++
	cont := c.out.Block("cont" + itoa(c.conts))
	c.b.BrIf(bit, c.trapBlock().To(), cont.To())
	c.b = cont
	return nil
}

func (c *fn) allocStack(in *vil.Inst) error {
	elem := in.Aux().Type
	if !elem.IsValid() {
		elem = in.Result().Type().Object()
	}
	f := elem.Formal()
	if f == nil {
		return c.fail(ErrType, in.Op(), "no element type")
	}
	size := types.Sizeof(f, types.DefaultTarget64)
	align := types.Alignof(f, types.DefaultTarget64)
	if size < 0 || align <= 0 {
		return c.fail(ErrType, in.Op(), elem.String())
	}
	c.def(in.Result(), c.b.Ptr.Alloc(uint64(size), uint64(align)))
	return nil
}

// objectHeaderSize is how far into an instance its first stored
// property is. Two words: what the object is, and how many references
// are held to it. The runtime that reads those words is compiled and
// linked by vcc, and this is the one place the compiler needs to agree
// with it about their size.
const objectHeaderWords = 2

// refElementAddr is a stored property of a class instance. A class is
// a reference, so this is arithmetic on that reference and not a load.
func (c *fn) refElementAddr(in *vil.Inst) error {
	obj, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	p, ok := obj.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the operand is not a reference")
	}
	field := memberName(in.Aux().Member)
	if field == "" {
		return c.fail(ErrUnsupported, in.Op(), "no field named")
	}
	off, ok := types.Offsetof(in.Args()[0].Type().Formal(), field, types.DefaultTarget64)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "no such field: "+field)
	}
	header := objectHeaderWords * types.DefaultTarget64.WordSize
	c.def(in.Result(), c.b.Ptr.Add(p, c.b.I64.Const(header+off)))
	return nil
}

// memberName takes the field out of a declaration reference, which SIL
// writes as #Type.name.
func memberName(member string) string {
	if i := strings.LastIndexByte(member, '.'); i >= 0 {
		return member[i+1:]
	}
	return member
}

func (c *fn) load(in *vil.Inst) error {
	addr, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	p, ok := addr.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the operand is not an address")
	}
	res := in.Result()
	r, ok := machine(res.Type())
	if !ok {
		return c.fail(ErrType, in.Op(), res.Type().String())
	}
	// A narrow type occupies its own width in memory and a whole
	// register once loaded, so the load says which extension it wants.
	if r.narrow() {
		ns := c.b.I32
		switch {
		case r.width == 8 && r.signed:
			c.def(res, ns.SLoad8(p))
		case r.width == 8:
			c.def(res, ns.ULoad8(p))
		case r.signed:
			c.def(res, ns.SLoad16(p))
		default:
			c.def(res, ns.ULoad16(p))
		}
		return nil
	}
	switch r.reg {
	case ir.TypeI32:
		c.def(res, c.b.I32.Load(p))
	case ir.TypeI64:
		c.def(res, c.b.I64.Load(p))
	case ir.TypeF32:
		c.def(res, c.b.F32.Load(p))
	case ir.TypeF64:
		c.def(res, c.b.F64.Load(p))
	case ir.TypePtr:
		c.def(res, c.b.Ptr.Load(p))
	case ir.TypeI1:
		// A Bool is one byte in memory and one bit in a register.
		c.def(res, c.b.I32.Ne(c.b.I32.ULoad8(p), c.b.I32.Const(0)))
	default:
		return c.fail(ErrType, in.Op(), r.reg.String())
	}
	return nil
}

func (c *fn) store(in *vil.Inst) error {
	args, err := c.operands(in)
	if err != nil {
		return err
	}
	p, ok := args[1].(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the destination is not an address")
	}
	r, ok := machine(in.Args()[0].Type())
	if !ok {
		return c.fail(ErrType, in.Op(), in.Args()[0].Type().String())
	}
	if r.narrow() {
		v := args[0].(ir.I32)
		if r.width == 8 {
			c.b.I32.Store8(v, p)
		} else {
			c.b.I32.Store16(v, p)
		}
		return nil
	}
	switch v := args[0].(type) {
	case ir.I32:
		c.b.I32.Store(v, p)
	case ir.I64:
		c.b.I64.Store(v, p)
	case ir.F32:
		c.b.F32.Store(v, p)
	case ir.F64:
		c.b.F64.Store(v, p)
	case ir.Ptr:
		c.b.Ptr.Store(v, p)
	case ir.I1:
		c.b.I32.Store8(c.b.I32.Select(v, c.b.I32.Const(1), c.b.I32.Const(0)), p)
	default:
		return c.fail(ErrType, in.Op(), "unstorable")
	}
	return nil
}

// refCount is a retain or a release. What they do belongs to the
// runtime, which vcc compiles and links; that they exist is all this
// package needs to know.
func (c *fn) refCount(in *vil.Inst, slot *ir.Callee, name string) error {
	v, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	p, ok := v.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the operand is not a reference")
	}
	if *slot == nil {
		*slot = c.l.out.ImportFunc(name, ir.NewSig().Param(ir.TypePtr)).NoUnwind()
	}
	c.b.Call(*slot, p)
	return nil
}

func (c *fn) apply(in *vil.Inst) error {
	callee, ok := c.refs[in.Args()[0]]
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "an indirect call")
	}
	args := make([]ir.Value, 0, len(in.Args())-1)
	for _, a := range in.Args()[1:] {
		got, err := c.operand(in, a)
		if err != nil {
			return err
		}
		args = append(args, got)
	}
	res := c.b.Call(callee, args...)
	if r := in.Result(); r != nil && !empty(r.Type()) && res.Len() > 0 {
		c.def(r, res.Value(0))
	}
	return nil
}

func (c *fn) br(in *vil.Inst) error {
	dest, ok := c.blocks[in.Aux().Dest]
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "no such block")
	}
	c.b.Br(dest.To(c.args(in.Aux().Args)...))
	return nil
}

func (c *fn) condBr(in *vil.Inst) error {
	cond, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	bit, ok := cond.(ir.I1)
	if !ok {
		return c.fail(ErrType, in.Op(), "the condition is not an i1")
	}
	then, tok := c.blocks[in.Aux().Dest]
	els, eok := c.blocks[in.Aux().Else]
	if !tok || !eok {
		return c.fail(ErrUnsupported, in.Op(), "no such block")
	}
	c.b.BrIf(bit,
		then.To(c.args(in.Aux().Args)...),
		els.To(c.args(in.Aux().ElseArgs)...))
	return nil
}

func (c *fn) ret(in *vil.Inst) error {
	if len(in.Args()) == 0 {
		c.b.Return()
		return nil
	}
	if v, ok := c.value(in.Args()[0]); ok {
		c.b.Return(v)
		return nil
	}
	// A function whose result holds nothing returns nothing: SIL still
	// writes `return %0` with the empty tuple, and there is no register.
	if empty(in.Args()[0].Type()) {
		c.b.Return()
		return nil
	}
	return c.fail(ErrUnsupported, in.Op(), "a result held in memory")
}
