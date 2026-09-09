package lower

import (
	"math"
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
		// A tuple's bytes are a struct's bytes -- see tupleImage --
		// so building one is building that struct. The empty tuple is
		// the one that holds nothing and becomes no register at all.
		if len(in.Args()) == 0 {
			return nil
		}
		return c.makeStruct(in)

	// A thin function given the shape of a thick one. With no context
	// to add, the value is the code address it already was -- see
	// machineOf's case for a signature, which is the other half of
	// this decision.
	// A thin metatype is nothing at run time, so naming one produces
	// nothing. Everything that would read it -- a parameter, a call's
	// argument -- asks empty() first and skips it.
	case vil.Metatype:
		return c.metatype(in)

	case vil.ClassMethod:
		return c.classMethod(in)

	// The three an existential needs. Its five words are the buffer,
	// then the metadata word, then the table -- Swift's layout, kept
	// so that the buffer is the size Swift makes it.
	case vil.InitExistentialAddr:
		return c.initExistential(in)
	case vil.CopyAddr:
		return c.copyAddr(in)
	case vil.PointerToAddress:
		// A pointer taken as an address. No bits move: what changes
		// is what the type system will let it be used for.
		c.forward(in.Result(), in.Args()[0])
		return nil
	case vil.IndexAddr:
		return c.indexAddr(in)
	case vil.StringLiteral:
		return c.stringLiteral(in)
	case vil.DestroyAddr:
		return c.destroyExistential(in)
	case vil.OpenExistentialAddr:
		return c.openExistential(in)
	case vil.WitnessMethod:
		return c.witnessMethod(in)

	case vil.ThinToThickFunction:
		return c.thinToThick(in)

	case vil.Enum:
		return c.makeEnum(in)

	case vil.TryApply:
		return c.tryApply(in)

	case vil.SwitchEnum:
		return c.switchEnum(in)

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

	case vil.FloatLiteral:
		return c.floatLiteral(in)

	// --- arithmetic ---

	case vil.BuiltinCall:
		return c.builtinCall(in)

	case vil.TupleExtract:
		return c.extractElement(in)

	case vil.DestructureTuple:
		return c.destructureTuple(in)

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

// floatLiteral materializes a floating-point constant from the bits
// SIL writes it as.
func (c *fn) floatLiteral(in *vil.Inst) error {
	res := in.Result()
	r, ok := machine(res.Type())
	if !ok {
		return c.fail(ErrType, in.Op(), res.Type().String())
	}
	bits := uint64(in.Aux().Int)
	switch r.reg {
	case ir.TypeF64:
		c.def(res, c.b.F64.Const(math.Float64frombits(bits)))
	case ir.TypeF32:
		c.def(res, c.b.F32.Const(float64(math.Float32frombits(uint32(bits)))))
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
	// A struct too wide for registers is built where it will live:
	// there is nowhere else to put it, and every reader of it reads
	// through the address.
	if _, yes := indirect(res.Type()); yes {
		return c.makeStructInMemory(in, res)
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
	// A field of a value that lives in memory is read out of it,
	// rather than picked from the registers it was taken apart into.
	if base, ok := c.mem[in.Args()[0]]; ok {
		st, ok := structOf(in.Args()[0].Type())
		if !ok || st == nil {
			return c.fail(ErrType, in.Op(), in.Args()[0].Type().String())
		}
		name := memberName(in.Aux().Member)
		for _, f := range st.Fields {
			if f != nil && f.Name == name {
				return c.loadFieldFrom(in, base, st, name, f.Type)
			}
		}
		return c.fail(ErrUnsupported, in.Op(), "no field "+name)
	}
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
	obj, ok := got.Value(0).(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the allocator did not return a pointer")
	}

	// The metadata word, which the runtime allocated and zeroed and
	// this fills in: the address of the class's dispatch table. It is
	// what a class_method loads, and it is why the answer depends on
	// what was allocated rather than on what the expression said.
	if cl, ok := f.Underlying().(*types.Class); ok {
		if g, ok := c.l.vtable[cl.Name]; ok {
			c.b.Ptr.Store(c.b.Ptr.GetAddr(g), obj)
		}
	}
	c.def(res, obj)
	return nil
}

// classMethod reads an implementation out of the receiver's table.
//
// Two loads: the table from the object's metadata word, then the row.
// The row's index comes from the static class -- the one the call was
// written against -- and is right for the dynamic class because every
// table down a chain repeats its base's rows in its base's order.
func (c *fn) classMethod(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	recv, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	obj, ok := recv.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the receiver is not a reference")
	}
	cl, ok := classOf(in.Args()[0].Type())
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "a method on something that is not a class")
	}
	rows, ok := c.l.slots[cl.Name]
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), cl.Name+" has no dispatch table")
	}
	i := -1
	for n, m := range rows {
		if m == in.Aux().Member {
			i = n
			break
		}
	}
	if i < 0 {
		return c.fail(ErrUnsupported, in.Op(), "no slot "+in.Aux().Member+" in "+cl.Name+"'s table")
	}
	table := c.b.Ptr.Load(obj)
	c.def(res, c.b.Ptr.Load(c.fieldAddr(table, int64(i)*8)))
	return nil
}

// classOf is the class a value's type is.
func classOf(t vil.Type) (*types.Class, bool) {
	if !t.IsValid() || t.Formal() == nil {
		return nil, false
	}
	cl, ok := t.Formal().Underlying().(*types.Class)
	return cl, ok
}

// makeEnum builds one case of an enum, which for a payload-free enum is
// its tag: the case's position among the ones the type declares.
//
// The position rather than a name, because that is what survives to a
// machine — and the same number switchEnum branches on, which is why
// both read it from the type rather than counting separately.
func (c *fn) makeEnum(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	if o, ok := optionalOf(res.Type()); ok {
		return c.makeOptional(in, res, o)
	}
	if e, ok := payloadEnumOf(res.Type()); ok {
		return c.makePayloadEnum(in, res, e)
	}
	if len(in.Args()) != 0 {
		return c.fail(ErrUnsupported, in.Op(), "an enum case that carries a value")
	}
	tag, ok := enumTag(res.Type(), in.Aux().Member)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "no case "+in.Aux().Member+" in "+res.Type().String())
	}
	r, ok := machine(res.Type())
	if !ok {
		return c.fail(ErrType, in.Op(), res.Type().String())
	}
	switch r.reg {
	case ir.TypeI32:
		c.def(res, c.b.I32.Const(tag))
	case ir.TypeI64:
		c.def(res, c.b.I64.Const(tag))
	default:
		return c.fail(ErrType, in.Op(), r.reg.String())
	}
	return nil
}

// switchEnum branches on the tag.
//
// A jump table rather than a chain of comparisons: the tags of an enum
// are 0, 1, 2 … by construction, which is the one shape a table is
// always right for. The default edge takes the cases the switch did not
// name, and there is always one — vil/gen adds it where the source
// named every case, because a table has to say where an unnamed
// selector goes even when the type says there are none.
func (c *fn) switchEnum(in *vil.Inst) error {
	if o, ok := optionalOf(in.Args()[0].Type()); ok {
		return c.switchOptional(in, o)
	}
	if e, ok := payloadEnumOf(in.Args()[0].Type()); ok {
		return c.switchPayloadEnum(in, e)
	}
	sel, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	tag, ok := sel.(ir.I32)
	if !ok {
		return c.fail(ErrType, in.Op(), "the tag is not an i32")
	}

	e, ok := enumOf(in.Args()[0].Type())
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), in.Args()[0].Type().String())
	}

	// One target per case, in tag order, so that the table is indexed
	// by the tag itself.
	targets := make([]ir.BlockTarget, len(e.Cases))
	var dflt *ir.Block
	for _, k := range in.Aux().Cases {
		dest, ok := c.blocks[k.Dest]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "no such block")
		}
		if k.Member == "" {
			dflt = dest
			continue
		}
		i, ok := enumTag(in.Args()[0].Type(), k.Member)
		if !ok || int(i) >= len(targets) {
			return c.fail(ErrUnsupported, in.Op(), "no case "+k.Member)
		}
		targets[i] = dest.To()
	}
	if dflt == nil {
		return c.fail(ErrUnsupported, in.Op(), "a switch with no default edge")
	}
	// A case the switch did not name goes where anything unnamed goes.
	for i := range targets {
		if targets[i].Block() == nil {
			targets[i] = dflt.To()
		}
	}
	c.b.BrTable(tag, targets, dflt.To())
	return nil
}

// enumOf is the enum a type is.
func enumOf(t vil.Type) (*types.Enum, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	e, ok := t.Formal().Underlying().(*types.Enum)
	return e, ok
}

// enumTag is a case's position among the ones its type declares, which
// is the value the case is represented by.
func enumTag(t vil.Type, member string) (int64, bool) {
	e, ok := enumOf(t)
	if !ok {
		return 0, false
	}
	name := memberName(member)
	for i, c := range e.Cases {
		if c != nil && c.Name == name {
			return int64(i), true
		}
	}
	return 0, false
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
	// Nothing to read, for the same reason there was nothing to
	// write: a value of empty type occupies no bytes, and everything
	// downstream skips it rather than reading a register for it.
	if res := in.Result(); res != nil && empty(res.Type()) {
		return nil
	}
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
	// Nothing to write. A value of empty type -- Void, the empty
	// tuple, a struct with no stored properties -- occupies no bytes,
	// so storing it is storing nothing rather than storing zero.
	if empty(in.Args()[0].Type()) {
		return nil
	}
	addr, err := c.operand(in, in.Args()[1])
	if err != nil {
		return err
	}
	p, ok := addr.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the destination is not an address")
	}
	src := in.Args()[0]

	// A value that already lives in memory is copied rather than
	// written: it is too wide for registers, so there is nothing to
	// write it out of. A five-word struct handed to a generic
	// function is this -- the callee takes it by address, and what
	// goes into the storage the call sets aside is its bytes.
	if from, ok := c.mem[src]; ok {
		size, yes := storageBytes(src.Type())
		if !yes {
			return c.fail(ErrType, in.Op(), "a value in memory with no size")
		}
		c.b.MemCpy(p, from, c.b.I64.Const(size))
		return nil
	}

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
	// A String or an Array is counted by Swift's own bridge object
	// release rather than by this compiler's. See string.go.
	if bridged(in.Args()[0].Type()) {
		return c.bridgeRefCount(in, in.Op() == vil.StrongRetain)
	}
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
	callee, direct := c.refs[in.Args()[0]]
	var through ir.Ptr
	if !direct {
		// A call through a value: a closure, or a function passed in.
		// The pointer is an operand like any other and the signature
		// comes from the value's own type, which is what keeps an
		// indirect call well typed.
		v, err := c.operand(in, in.Args()[0])
		if err != nil {
			return err
		}
		p, ok := v.(ir.Ptr)
		if !ok {
			return c.fail(ErrType, in.Op(), "the callee is not a pointer")
		}
		through = p
	}
	// A result that comes back by address needs somewhere to come
	// back to, and its address goes first -- the same place the
	// signature declares it.
	var out ir.Ptr
	callResult := in.Result()
	var wantsSRet bool
	if callResult != nil {
		if _, yes := outResult(in, callResult); yes {
			slot, ok := c.wide[callResult]
			if !ok {
				return c.fail(ErrIR, in.Op(), "no slot reserved for a wide result")
			}
			out, wantsSRet = slot, true
		}
	}

	args := make([]ir.Value, 0, len(in.Args()))
	if wantsSRet {
		args = append(args, out)
	}
	for _, a := range in.Args()[1:] {
		// An argument of empty type is not passed, matching the
		// signature that does not declare a register for it.
		if empty(a.Type()) {
			continue
		}
		// One too wide for registers is passed as its address.
		if _, yes := indirect(a.Type()); yes {
			p, ok := c.mem[a]
			if !ok {
				return c.fail(ErrUnsupported, in.Op(),
					"a struct passed by address that is not in memory")
			}
			args = append(args, p)
			continue
		}
		// A struct goes as the words it is passed in.
		regs, wide, err := c.gatherArg(in, a)
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
	// A witness call ends with the metadata and the table the witness
	// was found in. See witnessMethod, and the same two parameters in
	// lower.signature.
	if extra, ok := c.witnessExtra[in.Args()[0]]; ok {
		args = append(args, extra[0], extra[1])
	}
	var res ir.Results
	if direct {
		res = c.b.Call(callee, args...)
	} else {
		ft, err := c.calleeType(in, in.Args()[0])
		if err != nil {
			return err
		}
		res = c.b.CallInd(through, ft, args...)
	}

	// What came back by address is already where it belongs -- unless
	// it is a value that has a register, which is the `@out` case: a
	// generic function returns its result through the caller's
	// storage because the caller is the only one who knows how big it
	// is, and once it is back the caller knows exactly that. Array's
	// subscript getter hands an Int32 back through a four-byte slot,
	// and what the program asked for is the Int32.
	if wantsSRet {
		if _, wide := indirect(callResult.Type()); !wide {
			if r, ok := machine(callResult.Type()); ok {
				v, err := c.loadScalar(in, out, r)
				if err != nil {
					return c.fail(ErrType, in.Op(), err.Error())
				}
				c.def(callResult, v)
				return nil
			}
		}
		c.mem[callResult] = out
		return nil
	}
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
	// A result that comes back by address is copied into the storage
	// the caller set aside, and the function returns nothing: the
	// value never was in registers and there is nothing to put in
	// them.
	if c.hasSRet {
		src, ok := c.mem[in.Args()[0]]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "an indirect result that is not in memory")
		}
		size, _ := indirect(in.Args()[0].Type())
		c.b.MemCpy(c.sret, src, c.b.I64.Const(size))
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

// thinToThick turns a named function into a value.
//
// Until the conversion, a function_ref is a symbol rather than a
// register: a direct call names it and needs no address. This is where
// something other than a call asks, and the answer is the address --
// one register, because gen refuses a closure that captures and the
// context word Swift's thick function carries would always be null.
// machineOf's case for a signature is the other half of that, and both
// change together when partial_apply arrives.
func (c *fn) thinToThick(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	callee, ok := c.refs[in.Args()[0]]
	if !ok {
		// A thick function made from something that was already a
		// value: a reabstraction, which needs the context this does
		// not have.
		return c.fail(ErrUnsupported, in.Op(), "a function value that is not a reference to a declaration")
	}
	c.def(res, c.b.Ptr.GetAddr(callee))
	return nil
}

// calleeType is the VIR func typedef an indirect call names.
//
// A callind carries its signature rather than reading it off the
// callee, because there is no callee to read it off -- so the type of
// the value being called is what says how to call it, and this is
// where a Swift function type becomes one.
func (c *fn) calleeType(in *vil.Inst, v *vil.Value) (*ir.Type, error) {
	t := v.Type()
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, c.fail(ErrType, in.Op(), "the callee has no function type")
	}
	switch f := t.Formal().Underlying().(type) {
	case *types.Signature:
		// A closure or a function value: a Swift function type.
		ft, err := c.l.funcTypeOf(f)
		if err != nil {
			return nil, c.fail(ErrType, in.Op(), err.Error())
		}
		return ft, nil
	case *vil.FuncType:
		// A method loaded out of a dispatch table, which already
		// carries the lowered shape -- the receiver among the
		// parameters, in the place the convention puts it.
		ft, err := c.l.lowerFuncType(f)
		if err != nil {
			return nil, c.fail(ErrType, in.Op(), err.Error())
		}
		return ft, nil
	}
	return nil, c.fail(ErrType, in.Op(), "the callee is not a function")
}

// makeStructInMemory builds a struct that does not fit in registers.
//
// A slot of its own, and one store per field at the offset the layout
// gives it. Nothing about this is a copy of what the register path
// does: that one packs fields into words because a word is what
// crosses a call, and this one writes the memory image because the
// memory image is what crosses a call.
func (c *fn) makeStructInMemory(in *vil.Inst, res *vil.Value) error {
	slot, ok := c.wide[res]
	if !ok {
		return c.fail(ErrIR, in.Op(), "no slot reserved for a wide struct")
	}
	st, ok := structOf(res.Type())
	if !ok || st == nil {
		return c.fail(ErrType, in.Op(), res.Type().String())
	}
	args := in.Args()
	if len(args) != len(st.Fields) {
		return c.fail(ErrIR, in.Op(), "a struct built from the wrong number of fields")
	}
	for i, f := range st.Fields {
		if f == nil || f.Type == nil {
			return c.fail(ErrType, in.Op(), res.Type().String())
		}
		off, ok := types.Offsetof(st, f.Name, types.DefaultTarget64)
		if !ok {
			return c.fail(ErrType, in.Op(), "no offset for "+f.Name)
		}
		r, ok := machineOf(f.Type)
		if !ok {
			return c.fail(ErrType, in.Op(), whyNoRegister(vil.Object(f.Type)))
		}
		v, err := c.operand(in, args[i])
		if err != nil {
			return err
		}
		if err := c.storeScalar(in, v, c.fieldAddr(slot, off), r); err != nil {
			return err
		}
	}
	c.mem[res] = slot
	return nil
}

// loadFieldFrom reads one field out of a value that lives in memory.
func (c *fn) loadFieldFrom(in *vil.Inst, base ir.Ptr, st *types.Struct, name string, ft types.Type) error {
	off, ok := types.Offsetof(st, name, types.DefaultTarget64)
	if !ok {
		return c.fail(ErrType, in.Op(), "no offset for "+name)
	}
	r, ok := machineOf(ft)
	if !ok {
		return c.fail(ErrType, in.Op(), whyNoRegister(vil.Object(ft)))
	}
	v, err := c.loadScalar(in, c.fieldAddr(base, off), r)
	if err != nil {
		return c.fail(ErrType, in.Op(), err.Error())
	}
	c.def(in.Result(), v)
	return nil
}

// The layout of an existential, in bytes: three words of buffer, the
// metadata word, then the witness table.
const (
	existentialBuffer   = 0
	existentialMetadata = 3 * 8
	existentialWitness  = 4 * 8
	existentialSize     = 5 * 8
)

// initExistential writes the table into an existential and yields the
// address the value goes at.
//
// Which table follows from the concrete type and the protocol, so the
// instruction carries the type and the slot's own type carries the
// protocol -- nothing at the point of use has to name the
// conformance, which is what makes this the same three instructions
// SILGen writes.
func (c *fn) initExistential(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	slot, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	p, ok := slot.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "an existential that is not an address")
	}
	concrete := in.Aux().Type
	protos, ok := protocolsOf(in.Args()[0].Type())
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "an existential of no protocol")
	}
	// One table today: a value satisfying several protocols needs a
	// word for each, and the layout above has room for one. `Any` has
	// none at all, which is the four-word shape.
	if len(protos) > 1 {
		return c.fail(ErrUnsupported, in.Op(),
			"an existential of more than one protocol")
	}
	var table *ir.Global
	if len(protos) == 1 {
		table, ok = c.l.witness[typeNameOfType(concrete)+":"+protos[0]]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(),
				"no witness table for "+typeNameOfType(concrete)+": "+protos[0])
		}
	}
	if len(in.Args()) < 2 {
		return c.fail(ErrUnsupported, in.Op(),
			"an existential filled in without the metadata that says what is inside it")
	}
	meta, err := c.operand(in, in.Args()[1])
	if err != nil {
		return err
	}
	record, ok := meta.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "metadata that is not a pointer")
	}
	if table != nil {
		c.b.Ptr.Store(c.b.Ptr.GetAddr(table), c.fieldAddr(p, existentialWitness))
	}
	c.b.Ptr.Store(record, c.fieldAddr(p, existentialMetadata))
	c.def(res, c.fieldAddr(p, existentialBuffer))
	return nil
}

// The value witness table, which hangs one word below the metadata,
// and the offset of the flags word in it: eight function pointers,
// then size and stride, then the flags. swiftc reads the same two
// places -- `ldur x8, [x1,#-8]` then `ldr w8, [x8,#0x50]`.
const (
	valueWitnessOffset = -8
	valueWitnessFlags  = 0x50
	// valueWitnessDestroy is the second of the eight functions: what
	// ends the life of a value of the type.
	valueWitnessDestroy = 0x8
	// vwIsNonInline says the value did not fit the buffer and lives in
	// a heap box whose first word the buffer holds.
	vwIsNonInline = 1 << 17
	// vwAlignMask is the value's alignment, minus one, in the low
	// byte of the same word.
	vwAlignMask = 0xff
	// heapObjectSize is what a box's header takes before its value.
	heapObjectSize = 16
)

// openExistential is the address of the value inside one.
//
// Where that address is depends on what is inside: a value small
// enough for the buffer is in it, and one that is not is in a heap box
// whose pointer the buffer holds. Which of the two is a fact about
// the dynamic type, so it is read at run time out of the value
// witness table the metadata carries, exactly as swiftc's own
// __swift_project_boxed_opaque_existential_1 reads it.
//
// It is written without a branch because both answers are cheap and
// neither can fault: the box pointer is the first word of a buffer
// this function owns, so loading it is safe even when the value is
// inline and that word is part of the value itself.
func (c *fn) openExistential(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	p, err := c.addressOf(in, in.Args()[0])
	if err != nil {
		return err
	}
	buf := c.fieldAddr(p, existentialBuffer)
	meta := c.b.Ptr.Load(c.fieldAddr(p, existentialMetadata))
	vwt := c.b.Ptr.Load(c.fieldAddr(meta, valueWitnessOffset))
	flags := c.b.I64.ZExtI32(c.b.I32.Load(c.fieldAddr(vwt, valueWitnessFlags)))

	// The box, and where the value sits inside it: past the header,
	// rounded up to the value's own alignment.
	mask := c.b.I64.And(flags, c.b.I64.Const(vwAlignMask))
	off := c.b.I64.And(c.b.I64.Add(mask, c.b.I64.Const(heapObjectSize)),
		c.b.I64.Not(mask))
	boxed := c.b.Ptr.Add(c.b.Ptr.Load(buf), off)

	boxes := c.b.I64.Ne(c.b.I64.And(flags, c.b.I64.Const(vwIsNonInline)),
		c.b.I64.Const(0))
	c.def(res, c.b.Ptr.Select(boxes, boxed, buf))
	return nil
}

// destroyExistential ends the life of what is inside an existential.
//
// Which is two answers, and swiftc's own outlined
// __swift_destroy_boxed_opaque_existential_1 gives both: a value in
// the buffer is destroyed by its own destroy witness, the second
// function in the value witness table, called with the buffer and the
// metadata; a value in a box is let go of by releasing the box, and
// the box's own deinit destroys what is in it.
//
// It is the only place in this package that needs a branch of its
// own, because one arm is a call and the other is a different call.
func (c *fn) destroyExistential(in *vil.Inst) error {
	if len(in.Args()) == 0 {
		return c.fail(ErrUnsupported, in.Op(), "nothing to destroy")
	}
	if _, ok := existentialBytes(in.Args()[0].Type()); !ok {
		return c.fail(ErrUnsupported, in.Op(),
			"destroying something other than an existential by address")
	}
	p, err := c.addressOf(in, in.Args()[0])
	if err != nil {
		return err
	}
	buf := c.fieldAddr(p, existentialBuffer)
	meta := c.b.Ptr.Load(c.fieldAddr(p, existentialMetadata))
	vwt := c.b.Ptr.Load(c.fieldAddr(meta, valueWitnessOffset))
	flags := c.b.I64.ZExtI32(c.b.I32.Load(c.fieldAddr(vwt, valueWitnessFlags)))
	boxes := c.b.I64.Ne(c.b.I64.And(flags, c.b.I64.Const(vwIsNonInline)),
		c.b.I64.Const(0))

	c.conts++
	n := itoa(c.conts)
	inBox := c.out.Block("boxed" + n)
	inline := c.out.Block("inline" + n)
	done := c.out.Block("destroyed" + n)
	c.b.BrIf(boxes, inBox.To(), inline.To())

	c.b = inBox
	if c.l.swiftRelease == nil {
		c.l.swiftRelease = c.l.out.ImportFunc(c.l.sym("swift_release"),
			ir.NewSig().Param(ir.TypePtr))
	}
	c.b.Call(c.l.swiftRelease, c.b.Ptr.Load(buf))
	c.b.Br(done.To())

	c.b = inline
	c.b.CallInd(c.b.Ptr.Load(c.fieldAddr(vwt, valueWitnessDestroy)),
		c.l.destroyWitnessType(), buf, meta)
	c.b.Br(done.To())

	c.b = done
	return nil
}

// copyAddr copies what one address holds into another.
//
// The bytes and nothing else. A value that owns something would need
// its value witness to copy it -- a retain rather than a duplicated
// word -- and every value this reaches is one that owns nothing or
// one whose ownership has already been handed over: an existential
// going into an array is the caller's to begin with and the array's
// afterwards.
func (c *fn) copyAddr(in *vil.Inst) error {
	if len(in.Args()) != 2 {
		return c.fail(ErrUnsupported, in.Op(), "a copy with no source or no destination")
	}
	src, err := c.addressOf(in, in.Args()[0])
	if err != nil {
		return err
	}
	dst, err := c.addressOf(in, in.Args()[1])
	if err != nil {
		return err
	}
	size, ok := storageBytes(in.Args()[0].Type())
	if !ok {
		return c.fail(ErrType, in.Op(), "a value with no size")
	}
	c.b.MemCpy(dst, src, c.b.I64.Const(size))
	return nil
}

// addressOf is where a value lives, whether it arrived in a register
// holding its address or was left in memory by whatever produced it.
func (c *fn) addressOf(in *vil.Inst, v *vil.Value) (ir.Ptr, error) {
	if p, ok := c.mem[v]; ok {
		return p, nil
	}
	got, err := c.operand(in, v)
	if err != nil {
		return ir.Ptr{}, err
	}
	p, ok := got.(ir.Ptr)
	if !ok {
		return ir.Ptr{}, c.fail(ErrType, in.Op(), "a value that is not an address")
	}
	return p, nil
}

// witnessMethod reads an implementation out of the table an
// existential carries.
//
// Two loads, the same shape as a class_method: the table out of the
// value, then the row out of the table. The row's index is the
// requirement's place in the protocol, which is the same in every
// table for that protocol -- and has to be, because this knows the
// protocol and not the type.
func (c *fn) witnessMethod(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	if len(in.Args()) == 0 {
		return c.fail(ErrUnsupported, in.Op(),
			"a witness_method with nothing to look it up in")
	}
	p, err := c.addressOf(in, in.Args()[0])
	if err != nil {
		return err
	}
	chain, member, ok := splitMember(in.Aux().Member)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "a requirement with no protocol: "+in.Aux().Member)
	}
	protos := strings.Split(chain, ":")

	// The first word of a table is its conformance descriptor, so a
	// row is one past its place in the protocol. That is swiftc's
	// layout and not a choice: `x.v()` through `any P` loads [table,
	// #0x8] for the first requirement and [table, #0x10] for the
	// second.
	//
	// A requirement a protocol inherits rather than declares is one
	// step further along: the table the value carries holds the
	// inherited conformance's own table in a row, and the
	// implementation is in that one. swiftc's `x.b()` through
	// `any Sub` is exactly the two loads.
	table := c.b.Ptr.Load(c.fieldAddr(p, existentialWitness))
	for i := 0; i+1 < len(protos); i++ {
		n, err := c.witnessRow(in, protos[i], protos[i]+":"+protos[i+1])
		if err != nil {
			return err
		}
		table = c.b.Ptr.Load(c.fieldAddr(table, int64(n+1)*8))
	}
	last := protos[len(protos)-1]
	n, err := c.witnessRow(in, last, last+"."+member)
	if err != nil {
		return err
	}
	c.def(res, c.b.Ptr.Load(c.fieldAddr(table, int64(n+1)*8)))
	// A witness is called with two arguments the source never wrote:
	// the metadata for whatever is answering, and the table the call
	// came through. Both are in the existential, and this is where
	// they were read out of it.
	c.witnessExtra[res] = [2]ir.Value{
		c.b.Ptr.Load(c.fieldAddr(p, existentialMetadata)),
		table,
	}
	return nil
}

// witnessRow is a row's place in a protocol's table.
func (c *fn) witnessRow(in *vil.Inst, proto, row string) (int, error) {
	rows, ok := c.l.witnessRows[proto]
	if !ok {
		return 0, c.fail(ErrUnsupported, in.Op(),
			"nothing said what order "+proto+" lists its requirements in")
	}
	for i, r := range rows {
		if r == row {
			return i, nil
		}
	}
	return 0, c.fail(ErrUnsupported, in.Op(), "no row "+row+" in "+proto)
}

// hasAttribute reports whether an instruction carries a modifier.
func hasAttribute(attrs []string, want string) bool {
	for _, a := range attrs {
		if a == want {
			return true
		}
	}
	return false
}

// splitMember takes a requirement reference apart: P.v is the
// protocol and the name.
func splitMember(member string) (proto, name string, ok bool) {
	i := strings.LastIndexByte(member, '.')
	if i < 0 {
		return "", member, false
	}
	return member[:i], member[i+1:], true
}

// protocolsOf is the protocols an existential type names.
func protocolsOf(t vil.Type) ([]string, bool) {
	if !t.IsValid() || t.Formal() == nil {
		return nil, false
	}
	switch n := t.Formal().(type) {
	case *types.Existential:
		out := make([]string, 0, len(n.Protocols))
		for _, p := range n.Protocols {
			if p != nil {
				out = append(out, p.Name)
			}
		}
		return out, true
	case *types.Protocol:
		return []string{n.Name}, true
	}
	return nil, false
}

// typeNameOfType is a nominal type's own name.
func typeNameOfType(t vil.Type) string {
	if !t.IsValid() || t.Formal() == nil {
		return "?"
	}
	switch n := t.Formal().Underlying().(type) {
	case *types.Struct:
		return n.Name
	case *types.Class:
		return n.Name
	case *types.Enum:
		return n.Name
	}
	return t.Formal().String()
}

// metatype is a type named as a value.
//
// A thin one is nothing at run time: the type is known statically, so
// there are no bits and the instruction defines nothing -- which is
// what an initializer's metatype parameter is, and why it costs no
// register.
//
// A class's is a pointer, and it comes from a function. swiftc's own
// client code calls the type's metadata accessor with a request of
// zero and uses what comes back, because a class's metadata is not a
// constant the linker can relocate: it may have to be built the first
// time it is asked for.
func (c *fn) metatype(in *vil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	mt, ok := in.Aux().Type.Formal().(*vil.MetatypeType)
	if !ok || mt.Thin() {
		return nil
	}
	accessor := in.Aux().Name
	if accessor == "" {
		return c.fail(ErrUnsupported, in.Op(), "a metatype with no accessor to ask for it")
	}
	// A record another library exports rather than a function to
	// call: the metadata is at an offset inside it. See
	// MetadataGlobal.
	if hasAttribute(in.Aux().Attrs, "global") {
		g := c.l.metadataRecord(accessor)
		c.def(res, c.b.Ptr.Add(c.b.Ptr.GetAddr(g), c.b.I64.Const(in.Aux().Int)))
		return nil
	}
	// An accessor this module defines is called rather than imported:
	// the metadata for a type declared here is emitted here too.
	if f, ok := c.l.witnessFns[c.l.sym(accessor)]; ok {
		got := c.b.Call(f, c.b.I64.Const(0))
		p, ok := got.Value(0).(ir.Ptr)
		if !ok {
			return c.fail(ErrType, in.Op(),
				"a metadata accessor that did not return a pointer")
		}
		c.def(res, p)
		return nil
	}
	callee := c.l.metadataAccessor(accessor)
	got := c.b.Call(callee, c.b.I64.Const(0))
	p, ok := got.Value(0).(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "a metadata accessor that did not return a pointer")
	}
	c.def(res, p)
	return nil
}

// optionalOf is the Optional a value's type is, if it is one.
func optionalOf(t vil.Type) (*types.Optional, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	o, ok := t.Formal().Underlying().(*types.Optional)
	return o, ok
}

// makeOptional builds one of an Optional's two cases.
//
// Where it has a tag byte the value is the payload and then that
// byte, which is the struct optionalImage describes -- so the case
// with something in it is that struct built from the payload and a
// zero, and the empty case is the same struct with a one and a
// payload of zeros. swiftc writes the zeros too: `takeOpt(nil)` in
// its own code stores wzr over the payload before storing the tag.
//
// Where it has no tag byte the empty case took a representation the
// payload does not use. Only one such payload is lowered here, a
// reference, and the representation is null.
func (c *fn) makeOptional(in *vil.Inst, res *vil.Value, o *types.Optional) error {
	some := in.Aux().Member == optionalSomeCase
	image, tagged := optionalImage(o)
	if !tagged {
		if !some {
			// The empty case as one of the representations the
			// payload does not use. A reference's is null; see
			// machineOf, which refuses the ones this does not know.
			c.def(res, c.b.Ptr.Const())
			return nil
		}
		if len(in.Args()) != 1 {
			return c.fail(ErrUnsupported, in.Op(), "a case with no value to carry")
		}
		c.forward(res, in.Args()[0])
		return nil
	}

	// The payload's registers, or zeros where there is no payload.
	var parts []ir.Value
	if some {
		if len(in.Args()) != 1 {
			return c.fail(ErrUnsupported, in.Op(), "a case with no value to carry")
		}
		a := in.Args()[0]
		if inner, ok := c.multi[a]; ok {
			parts = append(parts, inner...)
		} else {
			got, err := c.operand(in, a)
			if err != nil {
				return err
			}
			parts = append(parts, got)
		}
	} else {
		zeros, err := c.zeroLeaves(in, o.Wrapped)
		if err != nil {
			return err
		}
		parts = zeros
	}
	tag := int64(0)
	if !some {
		tag = 1
	}
	parts = append(parts, c.b.I32.Const(tag))
	_ = image
	if len(parts) == 1 {
		c.def(res, parts[0])
		return nil
	}
	c.multi[res] = parts
	return nil
}

// zeroLeaves is a zero for every scalar a type is made of, which is
// what the empty case writes over the payload it does not have.
func (c *fn) zeroLeaves(in *vil.Inst, t types.Type) ([]ir.Value, error) {
	if st, ok := t.Underlying().(*types.Struct); ok {
		leaves, ok := structLeaves(st)
		if !ok {
			return nil, c.fail(ErrUnsupported, in.Op(),
				"an optional of "+t.String()+", whose layout this cannot take apart")
		}
		out := make([]ir.Value, 0, len(leaves))
		for _, l := range leaves {
			z, err := c.zeroOf(in, l.typ)
			if err != nil {
				return nil, err
			}
			out = append(out, z)
		}
		return out, nil
	}
	z, err := c.zeroOf(in, t)
	if err != nil {
		return nil, err
	}
	return []ir.Value{z}, nil
}

// zeroOf is the zero of one scalar type.
func (c *fn) zeroOf(in *vil.Inst, t types.Type) (ir.Value, error) {
	r, ok := machineOf(t)
	if !ok {
		return nil, c.fail(ErrType, in.Op(), "no register for "+t.String())
	}
	switch r.reg {
	case ir.TypeI1:
		return c.b.I1.Const(false), nil
	case ir.TypeI32:
		return c.b.I32.Const(0), nil
	case ir.TypeI64:
		return c.b.I64.Const(0), nil
	case ir.TypeF32:
		return c.b.F32.Const(0), nil
	case ir.TypeF64:
		return c.b.F64.Const(0), nil
	case ir.TypePtr:
		return c.b.Ptr.Const(), nil
	}
	return nil, c.fail(ErrType, in.Op(), r.reg.String())
}

// optionalSomeCase is what gen calls the case that carries a value.
const optionalSomeCase = "Optional.some"

// switchOptional branches on which case an optional holds.
//
// Where it has a tag byte the answer is that byte: swiftc's own code
// loads it and compares it with one, so this compares it with zero
// and goes the other way -- the case that carries something is tag
// zero, which is the order the cases are declared in. Where it has no
// tag byte the empty case is the payload's spare representation, and
// for the one this knows that is a null pointer.
//
// The payload goes to the destination as its argument, which is where
// SILGen puts it and where the binding reads it from.
func (c *fn) switchOptional(in *vil.Inst, o *types.Optional) error {
	v := in.Args()[0]
	var some, none *ir.Block
	for _, k := range in.Aux().Cases {
		dest, ok := c.blocks[k.Dest]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "no such block")
		}
		switch k.Member {
		case optionalSomeCase:
			some = dest
		case optionalNoneCase, "":
			none = dest
		default:
			return c.fail(ErrUnsupported, in.Op(), "no case "+k.Member+" in an optional")
		}
	}
	if some == nil || none == nil {
		return c.fail(ErrUnsupported, in.Op(), "an optional switched on fewer than both cases")
	}

	if _, tagged := optionalImage(o); !tagged {
		// The payload is the whole value, and the empty case is its
		// spare representation.
		got, err := c.operand(in, v)
		if err != nil {
			return err
		}
		p, ok := got.(ir.Ptr)
		if !ok {
			return c.fail(ErrType, in.Op(),
				"an optional with no tag byte whose payload is not a reference")
		}
		isSome := c.b.Ptr.Ne(p, c.b.Ptr.Const())
		c.b.BrIf(isSome, some.To(p), none.To())
		return nil
	}

	parts, ok := c.multi[v]
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "an optional this function did not take apart")
	}
	if len(parts) < 2 {
		return c.fail(ErrUnsupported, in.Op(), "an optional with no tag to switch on")
	}
	// The payload is everything before the tag. A block argument is
	// one register, so only a payload of one is passed on -- gen
	// refuses the rest before it gets here.
	if len(parts) != 2 {
		return c.fail(ErrUnsupported, in.Op(),
			"an optional whose payload is more than one register")
	}
	tag, ok := c.toWord(parts[1], 8)
	if !ok {
		return c.fail(ErrType, in.Op(), "a tag this cannot read")
	}
	isSome := c.b.I64.Eq(tag, c.b.I64.Const(0))
	c.b.BrIf(isSome, some.To(parts[0]), none.To())
	return nil
}

// optionalNoneCase is what gen calls the case that carries nothing.
const optionalNoneCase = "Optional.none"

// extractElement reads one element out of a tuple.
//
// Two kinds of tuple reach this. A builtin's results are several
// registers with one element each, and the index is the register. A
// tuple the program wrote is a struct -- see tupleImage -- and the
// index names a field whose leaves are a window onto the parts, the
// way struct_extract reads one.
func (c *fn) extractElement(in *vil.Inst) error {
	v := in.Args()[0]
	i := int(in.Aux().Int)
	if i < 0 {
		return c.fail(ErrUnsupported, in.Op(), "element out of range")
	}
	parts, multi := c.multi[v]
	if lo, hi, ok := fieldLeaves(v.Type(), itoa(i)); ok {
		if !multi {
			// Every element but one holds nothing, so the whole
			// tuple is the one register that element is in.
			got, err := c.operand(in, v)
			if err != nil {
				return err
			}
			c.def(in.Result(), got)
			return nil
		}
		if lo >= len(parts) || hi > len(parts) {
			return c.fail(ErrUnsupported, in.Op(), "element out of range")
		}
		if hi-lo == 1 {
			c.def(in.Result(), parts[lo])
			return nil
		}
		c.multi[in.Result()] = parts[lo:hi]
		return nil
	}
	if !multi {
		return c.fail(ErrUnsupported, in.Op(), "a tuple this function did not take apart")
	}
	if i >= len(parts) {
		return c.fail(ErrUnsupported, in.Op(), "element out of range")
	}
	c.def(in.Result(), parts[i])
	return nil
}

// destructureTuple takes a tuple apart into all of its elements at
// once, which is tuple_extract for every element and nothing more:
// the registers are already there, and this is which of them each
// element owns.
func (c *fn) destructureTuple(in *vil.Inst) error {
	v := in.Args()[0]
	parts, multi := c.multi[v]
	for i, res := range in.Results() {
		if res == nil {
			continue
		}
		lo, hi, ok := fieldLeaves(v.Type(), itoa(i))
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "a tuple with no element "+itoa(i))
		}
		if !multi {
			// Every element but one holds nothing, so the whole tuple
			// is the one register that element is in.
			got, err := c.operand(in, v)
			if err != nil {
				return err
			}
			c.def(res, got)
			continue
		}
		if lo >= len(parts) || hi > len(parts) {
			return c.fail(ErrUnsupported, in.Op(), "element out of range")
		}
		if hi-lo == 1 {
			c.def(res, parts[lo])
			continue
		}
		c.multi[res] = parts[lo:hi]
	}
	return nil
}

// tryApply calls a function that may fail and takes the two edges out
// of it.
//
// The call is an ordinary one whose signature brings an extra result
// back in the error register; what makes it a terminator is that the
// register decides where control goes. A zero there is a call that
// did not fail, which is why the caller clears it first -- the callee
// writes it only on the path that does.
func (c *fn) tryApply(in *vil.Inst) error {
	var normal, failed *ir.Block
	for _, k := range in.Aux().Cases {
		dest, ok := c.blocks[k.Dest]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "no such block")
		}
		switch k.Member {
		case "normal":
			normal = dest
		case "error":
			failed = dest
		}
	}
	if normal == nil || failed == nil {
		return c.fail(ErrUnsupported, in.Op(), "a try with fewer than both edges")
	}

	callee, ok := c.refs[in.Args()[0]]
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "a try through something other than a function")
	}
	var args []ir.Value
	for _, a := range in.Args()[1:] {
		regs, wide, err := c.gatherArg(in, a)
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
	if res.Len() < 1 {
		return c.fail(ErrIR, in.Op(), "a call that may fail returned nothing at all")
	}
	// The error is the last result, which is where the signature put
	// it; everything before it is the value the normal edge carries.
	errValue, ok := res.Value(res.Len() - 1).(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "an error that is not a reference")
	}
	var carried []ir.Value
	for i := 0; i < res.Len()-1; i++ {
		carried = append(carried, res.Value(i))
	}
	threw := c.b.Ptr.Ne(errValue, c.b.Ptr.Const())
	c.b.BrIf(threw, failed.To(), normal.To(carried...))
	return nil
}
