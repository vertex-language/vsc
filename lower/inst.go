package lower

import (
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/runtime"
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
		return c.makeStruct(in)

	case vil.StructExtract:
		return c.extractField(in)

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

	case vil.StructElementAddr:
		return c.structElementAddr(in)

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

	case vil.AllocRef:
		return c.allocRef(in)

	case vil.AllocStack:
		// The slot was reserved in the entry block before any block
		// was walked, because VIR admits a frame allocation there and
		// nowhere else. See allocSlots.
		return nil
	case vil.Load:
		return c.load(in)
	case vil.Store:
		return c.store(in)

	// --- lifetime, now that it is calls ---

	case vil.StrongRetain:
		return c.refCount(in, &c.l.retain, runtime.Retain)
	case vil.StrongRelease:
		return c.refCount(in, &c.l.release, runtime.Release)

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
		if _, isAggregate := c.multi[v]; isAggregate {
			// Several registers where one is wanted. It is not that
			// the value is missing, it is that this instruction has
			// no way to take it whole — which needs a layout and an
			// ABI rather than a register.
			return nil, c.fail(ErrUnsupported, in.Op(),
				"a struct of more than one field, where one value is wanted")
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

// makeStruct builds an aggregate out of the registers its fields are
// in.
//
// A struct of one field is that field: Int is a struct around a
// Builtin.Int64 and Bool around a Builtin.Int1, and the register does
// not know the difference. Nothing is emitted and the result is an
// alias.
//
// More than one field is more than one register, which is the same
// shape an overflow-reporting builtin already produces — a VIL value
// standing for several VIR ones. Nothing is emitted for this either:
// the fields were computed where they were written, and the struct is
// the list of them.
//
// What this does not do is give the aggregate a place in memory or a
// way to cross a call boundary. Both need a layout and an ABI, and
// the two have to agree with what a C caller expects — so a struct
// that reaches a parameter, a result, or a store is refused by name
// where it gets there rather than passed in a shape invented here.
func (c *fn) makeStruct(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	switch len(in.Args()) {
	case 0:
		// A struct with nothing in it holds nothing, like the empty
		// tuple a void function returns.
		return nil
	case 1:
		c.forward(res, in.Args()[0])
		return nil
	}
	// Each operand contributes its own scalars: one for a field that
	// is a scalar, and a struct's whole list for a field that is a
	// struct. The result is the flat list, in layout order.
	parts := make([]ir.Value, 0, len(in.Args()))
	for _, a := range in.Args() {
		if inner, ok := c.multi[a]; ok {
			parts = append(parts, inner...)
			continue
		}
		got, err := c.operand(in, a)
		if err != nil {
			return err
		}
		parts = append(parts, got)
	}
	if len(parts) == 1 {
		c.def(res, parts[0])
		return nil
	}
	c.multi[res] = parts
	return nil
}

// extractField takes one register back out of an aggregate.
//
// Which register is the field's position among the struct's fields,
// read from the type rather than counted at the instruction: SIL
// names the field and the name is what has to be resolved.
func (c *fn) extractField(in *vil.Inst) error {
	base := in.Args()[0]
	parts, ok := c.multi[base]
	if !ok {
		// One field, or a value that is already just its field.
		c.forward(in.Result(), base)
		return nil
	}
	lo, hi, ok := fieldLeaves(base.Type(), in.Aux().Member)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "no field "+in.Aux().Member+" in "+base.Type().String())
	}
	if hi > len(parts) {
		return c.fail(ErrUnsupported, in.Op(), "field "+in.Aux().Member+" is past the end of the value")
	}
	res := in.Result()
	// One scalar is a value; several are a struct, and a struct is
	// the window onto the parts that are already there.
	if hi-lo == 1 {
		c.def(res, parts[lo])
		return nil
	}
	c.multi[res] = parts[lo:hi]
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

// objectHeaderWords is how far into an instance its first stored
// property is. Two words: what the object is, and how many references
// are held to it.
//
// The runtime reads those words, and runtime/ writes the same number
// down. It is the one thing the compiler and the runtime have to
// agree about, and they agree by both naming it rather than by one
// of them guessing.
const objectHeaderWords = runtime.HeaderWords

// allocRef makes an instance of a class.
//
// The size is the header plus the stored properties, and the runtime
// is what asks the platform for the memory — so this is a call, and
// the only thing this package decides is how big to ask for. The
// reference comes back with a count of one, which the caller holds.
func (c *fn) allocRef(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	t := in.Aux().Type
	if !t.IsValid() {
		t = res.Type().Object()
	}
	f := t.Formal()
	if f == nil {
		return c.fail(ErrType, in.Op(), "no instance type")
	}
	// The instance's size, not the value's: a class value is one
	// word, because it is a reference, and asking the allocator for
	// one word would give every instance eight bytes to hold whatever
	// it declared.
	size, ok := types.InstanceSizeof(f, types.DefaultTarget64)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), t.String()+": no layout for its stored properties")
	}
	if c.l.alloc == nil {
		c.l.alloc = c.l.out.ImportFunc(c.l.sym(runtime.Alloc),
			ir.NewSig().Param(ir.TypeI64).Ret(ir.TypePtr)).NoUnwind()
	}
	got := c.b.Call(c.l.alloc, c.b.I64.Const(size))
	if got.Len() == 0 {
		return c.fail(ErrIR, in.Op(), "the allocator returned nothing")
	}
	c.def(res, got.Value(0))
	return nil
}

// structElementAddr is a stored property inside a struct's own
// storage: arithmetic on the address of the struct, with no header to
// step over because a struct has none. That is the whole difference
// from refElementAddr — a class instance begins with what the runtime
// needs to know about it, and a struct begins with its first field.
func (c *fn) structElementAddr(in *vil.Inst) error {
	base, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	p, ok := base.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the operand is not an address")
	}
	field := memberName(in.Aux().Member)
	if field == "" {
		return c.fail(ErrUnsupported, in.Op(), "no field named")
	}
	off, ok := types.Offsetof(in.Args()[0].Type().Formal(), field, types.DefaultTarget64)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "no such field: "+field)
	}
	c.def(in.Result(), c.fieldAddr(p, off))
	return nil
}

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

	// A struct comes back one field at a time, from the offsets the
	// layout gave them, into the registers the body reads.
	if ls, ok := leavesOf(res.Type()); ok && len(ls) > 1 {
		fields := make([]ir.Value, 0, len(ls))
		for _, l := range ls {
			fr, ok := machineOf(l.typ)
			if !ok {
				return c.fail(ErrUnsupported, in.Op(),
					"a field of type "+l.typ.String()+", which has no register")
			}
			got, err := c.loadScalar(in, c.fieldAddr(p, l.offset), fr)
			if err != nil {
				return err
			}
			fields = append(fields, got)
		}
		c.multi[res] = fields
		return nil
	}

	r, ok := machine(res.Type())
	if !ok {
		return c.fail(ErrType, in.Op(), res.Type().String())
	}
	got, err := c.loadScalar(in, p, r)
	if err != nil {
		return err
	}
	c.def(res, got)
	return nil
}

// loadScalar reads one register from an address.
func (c *fn) loadScalar(in *vil.Inst, p ir.Ptr, r repr) (ir.Value, error) {
	// A narrow type occupies its own width in memory and a whole
	// register once loaded, so the load says which extension it wants.
	if r.narrow() {
		ns := c.b.I32
		switch {
		case r.width == 8 && r.signed:
			return ns.SLoad8(p), nil
		case r.width == 8:
			return ns.ULoad8(p), nil
		case r.signed:
			return ns.SLoad16(p), nil
		default:
			return ns.ULoad16(p), nil
		}
	}
	switch r.reg {
	case ir.TypeI32:
		return c.b.I32.Load(p), nil
	case ir.TypeI64:
		return c.b.I64.Load(p), nil
	case ir.TypeF32:
		return c.b.F32.Load(p), nil
	case ir.TypeF64:
		return c.b.F64.Load(p), nil
	case ir.TypePtr:
		return c.b.Ptr.Load(p), nil
	case ir.TypeI1:
		// A Bool is one byte in memory and one bit in a register.
		return c.b.I32.Ne(c.b.I32.ULoad8(p), c.b.I32.Const(0)), nil
	default:
		return nil, c.fail(ErrType, in.Op(), r.reg.String())
	}
}

func (c *fn) store(in *vil.Inst) error {
	addr, err := c.operand(in, in.Args()[1])
	if err != nil {
		return err
	}
	p, ok := addr.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the destination is not an address")
	}
	src := in.Args()[0]

	// A struct held in registers is written one field at a time, each
	// at the offset the layout gives it. Nothing puts the whole value
	// anywhere: there is no register wide enough, and going through
	// the packed word form would write padding the layout does not
	// have.
	if fields, ls, ok := c.spread(src); ok {
		for i, l := range ls {
			r, ok := machineOf(l.typ)
			if !ok {
				return c.fail(ErrUnsupported, in.Op(),
					"a field of type "+l.typ.String()+", which has no register")
			}
			if err := c.storeScalar(in, fields[i], c.fieldAddr(p, l.offset), r); err != nil {
				return err
			}
		}
		return nil
	}

	got, err := c.operand(in, src)
	if err != nil {
		return err
	}
	r, ok := machine(src.Type())
	if !ok {
		return c.fail(ErrType, in.Op(), src.Type().String())
	}
	return c.storeScalar(in, got, p, r)
}

// spread is the registers a struct's scalars are held in, paired with
// where each one sits.
func (c *fn) spread(v *vil.Value) ([]ir.Value, []leaf, bool) {
	ls, ok := leavesOf(v.Type())
	if !ok || len(ls) < 2 {
		return nil, nil, false
	}
	fields, ok := c.multi[v]
	if !ok || len(fields) != len(ls) {
		return nil, nil, false
	}
	return fields, ls, true
}

// fieldAddr is the address of something at an offset.
func (c *fn) fieldAddr(p ir.Ptr, off int64) ir.Ptr {
	if off == 0 {
		return p
	}
	return c.b.Ptr.Add(p, c.b.I64.Const(off))
}

// storeScalar writes one register to an address, in the width the
// value's declared type occupies in memory.
func (c *fn) storeScalar(in *vil.Inst, v ir.Value, p ir.Ptr, r repr) error {
	if r.narrow() {
		n, ok := v.(ir.I32)
		if !ok {
			return c.fail(ErrType, in.Op(), "a narrow value not in an i32 register")
		}
		if r.width == 8 {
			c.b.I32.Store8(n, p)
		} else {
			c.b.I32.Store16(n, p)
		}
		return nil
	}
	switch v := v.(type) {
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
// runtime, which runtime/ builds as a VIR module of its own; that
// they exist is all this package needs to know.
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
		*slot = c.l.out.ImportFunc(c.l.sym(name), ir.NewSig().Param(ir.TypePtr)).NoUnwind()
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
		// A struct goes as the words it is passed in.
		regs, wide, err := c.gather(in, a)
		if err != nil {
			return err
		}
		if wide {
			args = append(args, regs...)
			continue
		}
		got, err := c.operand(in, a)
		if err != nil {
			return err
		}
		args = append(args, got)
	}
	res := c.b.Call(callee, args...)

	r := in.Result()
	if r == nil || empty(r.Type()) || res.Len() == 0 {
		return nil
	}
	// A struct comes back in words too, and the caller wants fields.
	if n, wide := directWords(r.Type()); wide {
		if res.Len() < n {
			return c.fail(ErrIR, in.Op(), "the call returned fewer registers than the struct is passed in")
		}
		regs := make([]ir.Value, 0, n)
		for i := 0; i < n; i++ {
			regs = append(regs, res.Value(i))
		}
		return c.spreadInto(r, regs)
	}
	c.def(r, res.Value(0))
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
	// A struct is returned in the words it is passed in.
	regs, wide, err := c.gather(in, in.Args()[0])
	if err != nil {
		return err
	}
	if wide {
		c.b.Return(regs...)
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
