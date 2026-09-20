package lower

import (
	"fmt"
	"math"
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// inst translates a single SIL instruction into VIR.
func (c *fn) inst(in *sil.Inst) error {
	switch in.Op() {

	// --- the ones that cost nothing ---

	case sil.DebugValue:
		if v, ok := c.value(in.Args()[0]); ok && in.Aux().Name != "" && v.Def() != nil {
			if v.Def().Name() == "" {
				v.Def().SetName(in.Aux().Name)
			}
		}
		return nil

	case sil.Struct:
		return c.makeStruct(in)

	case sil.StructExtract:
		return c.extractField(in)

	case sil.BeginAccess:
		// Where exclusivity is enforced is a question this package is
		// downstream of: by now the checking either happened or was
		// found unnecessary, and the address is the address.
		c.forward(in.Result(), in.Args()[0])
		return nil

	case sil.EndAccess, sil.DeallocStack, sil.EndLifetime, sil.ExtendLifetime:
		return nil

	case sil.Tuple:
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
	case sil.Metatype:
		return c.metatype(in)

	case sil.GlobalAddr:
		return c.globalAddr(in)

	case sil.ClassMethod:
		return c.classMethod(in)

	// The three an existential needs. Its five words are the buffer,
	// then the metadata word, then the table -- Swift's layout, kept
	// so that the buffer is the size Swift makes it.
	case sil.InitExistentialAddr:
		return c.initExistential(in)
	case sil.CopyAddr:
		return c.copyAddr(in)
	case sil.PointerToAddress, sil.AddressToPointer:
		// An address and a pointer are one register. No bits move in
		// either direction: what changes is what the type system will
		// let it be used for.
		c.forward(in.Result(), in.Args()[0])
		return nil
	case sil.IndexAddr:
		return c.indexAddr(in)
	case sil.StringLiteral:
		return c.stringLiteral(in)
	case sil.DestroyAddr:
		return c.destroyExistential(in)
	case sil.OpenExistentialAddr:
		return c.openExistential(in)
	case sil.WitnessMethod:
		return c.witnessMethod(in)

	case sil.ThinToThickFunction:
		return c.thinToThick(in)
	case sil.PartialApply:
		return c.partialApply(in)
	case sil.AllocBox:
		return c.allocBox(in)
	case sil.ProjectBox:
		return c.projectBox(in)

	case sil.Enum:
		return c.makeEnum(in)

	case sil.TryApply:
		return c.tryApply(in)

	case sil.SwitchEnum:
		return c.switchEnum(in)

	case sil.StructElementAddr:
		return c.structElementAddr(in)

	case sil.UncheckedTakeEnumDataAddr:
		return c.takeEnumDataAddr(in)

	case sil.RefElementAddr:
		return c.refElementAddr(in)

	case sil.FunctionRef:
		callee, ok := c.l.callee[in.Aux().Name]
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "no such function: "+in.Aux().Name)
		}
		c.refs[in.Result()] = callee
		c.refNames[in.Result()] = in.Aux().Name
		return nil

	// --- literals ---

	case sil.IntegerLiteral:
		// Swift passes a with_overflow builtin a third operand saying
		// whether the overflow should be reported. VIR asks the
		// question with a separate instruction and has nothing to do
		// with the answer, so a literal that exists only to be that
		// operand is never materialized.
		if onlyOverflowFlag(in.Result()) {
			return nil
		}
		return c.integerLiteral(in)

	case sil.FloatLiteral:
		return c.floatLiteral(in)

	// --- arithmetic ---

	case sil.BuiltinCall:
		return c.builtinCall(in)

	case sil.TupleExtract:
		return c.extractElement(in)

	case sil.DestructureTuple:
		return c.destructureTuple(in)

	case sil.CondFail:
		return c.condFail(in)

	// --- memory ---

	case sil.AllocRef:
		return c.allocRef(in)

	case sil.AllocStack:
		// The slot was reserved in the entry block before any block
		// was walked, because VIR admits a frame allocation there and
		// nowhere else. See allocSlots.
		return c.zeroSlot(in)
	case sil.Load:
		return c.load(in)
	case sil.Store:
		return c.store(in)

	// --- lifetime, now that it is calls ---

	case sil.StrongRetain:
		return c.refCount(in, &c.l.retain, stdlib.Retain)
	case sil.StrongRelease:
		return c.refCount(in, &c.l.release, stdlib.Release)

	// --- calls and control flow ---

	case sil.Apply:
		return c.apply(in)
	case sil.Br:
		return c.br(in)
	case sil.CondBr:
		return c.condBr(in)
	case sil.Return:
		return c.ret(in)
	case sil.Throw:
		return c.throwInst(in)
	case sil.Unreachable:
		c.b.Trap()
		return nil
	}
	return c.fail(ErrUnsupported, in.Op(), "")
}

// asyncReturn ends an async function the way Swift ends one.
//
// There is nowhere to return to. The function may have been resumed on a
// thread that never called it, and its caller's frame is long gone, so
// the answer goes forward rather than back: the context's second word is
// the continuation to run next, and the result is its argument.
//
//	resume := ctx->resumeParent
//	tail_call resume(ctx, results...)
//
// It is a tail call and not a call because these chain -- every async
// return in a program is one of them -- and a frame each would be a
// recursion as deep as the program's call graph.
func (c *fn) asyncReturn(in *sil.Inst) error {
	if c.ctx.IsZero() {
		return c.fail(ErrUnsupported, in.Op(), "an async function with no context")
	}
	// The continuation, from the second word of the context.
	resume := c.b.Ptr.Load(c.b.Ptr.Add(c.ctx, c.b.I64.Const(8)))

	// A result too wide for registers was never in them: it goes into
	// the storage the caller set aside, and the continuation is handed
	// nothing. Swift does the same -- an async function with an
	// indirect result takes the pointer as a parameter like any other.
	throws := c.srcThrows()
	if c.hasSRet && len(in.Args()) > 0 && !empty(in.Args()[0].Type()) {
		if err := c.retIndirect(in); err != nil {
			return err
		}
		args := []ir.Value{c.ctx}
		if throws {
			args = append(args, c.b.Ptr.Const())
		}
		c.b.TailCallInd(resume, c.l.continuationType(nil, throws), args...)
		return nil
	}

	args := []ir.Value{c.ctx}
	for _, a := range in.Args() {
		if empty(a.Type()) {
			continue
		}
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

	// Nothing was thrown, and a function that may throw says so with a
	// null error rather than by leaving the register alone.
	results := args[1:]
	if throws {
		args = append(args, c.b.Ptr.Const())
	}
	c.b.TailCallInd(resume, c.l.continuationType(results, throws), args...)
	return nil
}

// srcThrows reports whether the function being lowered may fail.
func (c *fn) srcThrows() bool {
	t := c.src.Type()
	return t != nil && t.ErrorType.IsValid()
}

// operand is the register an instruction reads, and an error when
// there is not one -- which means either a value this package holds in
// memory, or a function reference used somewhere other than a call.
func (c *fn) operand(in *sil.Inst, v *sil.Value) (ir.Value, error) {
	got, ok := c.value(v)
	if !ok {
		if _, isRef := c.refs[v]; isRef {
			return nil, c.fail(ErrUnsupported, in.Op(), "a function reference used other than as a callee")
		}
		if _, isAggregate := c.parts(v); isAggregate {
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

func (c *fn) operands(in *sil.Inst) ([]ir.Value, error) {
	return c.operandsOf(in, in.Args())
}

func (c *fn) operandsOf(in *sil.Inst, vs []*sil.Value) ([]ir.Value, error) {
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
func onlyOverflowFlag(v *sil.Value) bool {
	if v == nil || len(v.Uses()) == 0 {
		return false
	}
	for _, u := range v.Uses() {
		if u.Op() != sil.BuiltinCall || len(u.Args()) < 3 || u.Args()[2] != v {
			return false
		}
		if verb, _, ok := splitBuiltin(u.Aux().Name); !ok || !strings.HasSuffix(verb, "_with_overflow") {
			return false
		}
	}
	return true
}

func (c *fn) integerLiteral(in *sil.Inst) error {
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
func (c *fn) floatLiteral(in *sil.Inst) error {
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

func (c *fn) builtinCall(in *sil.Inst) error {
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

// makeStruct lowers a struct value from its field registers or delegates
// to makeStructInMemory if the struct exceeds direct register passing limits.
func (c *fn) makeStruct(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	// Wide structs are constructed directly in memory.
	if _, yes := indirect(res.Type()); yes {
		return c.makeStructInMemory(in, res)
	}
	// A field of no bytes has no leaf -- see appendLeaves -- so it
	// contributes no register, and a struct of one field that holds
	// something and others that hold nothing is that one field.
	var held []*sil.Value
	for _, a := range in.Args() {
		if a != nil && a.Type().Formal() != nil && types.Sizeof(a.Type().Formal(), types.DefaultTarget64) == 0 {
			continue
		}
		held = append(held, a)
	}
	switch len(held) {
	case 0:
		return nil
	case 1:
		c.forward(res, held[0])
		return nil
	}
	// Flatten operands into scalar registers in layout order.
	parts := make([]ir.Value, 0, len(held))
	for _, a := range held {
		if inner, ok := c.parts(a); ok {
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

// holdsNothing reports whether a value of t occupies no bytes.
func holdsNothing(t sil.Type) bool {
	return t.IsValid() && !t.IsAddress() && t.Formal() != nil &&
		types.Sizeof(t.Formal(), types.DefaultTarget64) == 0
}

// defNothing defines a value of a type that holds no bytes: the zero of
// whatever register the type has on its own, or nothing where it has none.
// A one-case enum is its only case, whose tag is 0.
func (c *fn) defNothing(in *sil.Inst, res *sil.Value) error {
	if _, ok := machine(res.Type()); !ok || empty(res.Type()) || res.Type().Formal() == nil {
		return nil
	}
	z, err := c.zeroOf(in, res.Type().Formal())
	if err != nil {
		return err
	}
	c.def(res, z)
	return nil
}

// extractField extracts a single field from a struct register aggregate or memory slot.
func (c *fn) extractField(in *sil.Inst) error {
	// A field of no bytes is in no register and no memory. See defNothing.
	if res := in.Result(); res != nil && holdsNothing(res.Type()) {
		return c.defNothing(in, res)
	}
	// Read field from memory if the struct is address-backed.
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
	parts, ok := c.parts(base)
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
func (c *fn) condFail(in *sil.Inst) error {
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
	c.b.BrIf(bit, c.trapBlockFor(in.Aux().Text).To(), cont.To())
	c.b = cont
	return nil
}

// stackBytes is how much room an alloc_stack wants, and what it has to
// be aligned to.
func (c *fn) stackBytes(in *sil.Inst) (size, align int64, err error) {
	elem := in.Aux().Type
	if !elem.IsValid() {
		elem = in.Result().Type().Object()
	}
	f := elem.Formal()
	if f == nil {
		return 0, 0, c.fail(ErrType, in.Op(), "no element type")
	}
	size = types.Sizeof(f, types.DefaultTarget64)
	align = types.Alignof(f, types.DefaultTarget64)
	if size < 0 || align <= 0 {
		return 0, 0, c.fail(ErrType, in.Op(), elem.String())
	}
	return size, align, nil
}

// zeroSlot clears a slot marked zeroed each time its declaration is
// reached: a variable declared without a value is first assigned, which
// lets go of what was there, and zero is what letting go of does nothing.
func (c *fn) zeroSlot(in *sil.Inst) error {
	zeroed := false
	for _, a := range in.Aux().Attrs {
		zeroed = zeroed || a == "zeroed"
	}
	if !zeroed {
		return nil
	}
	got, ok := c.value(in.Result())
	slot, isPtr := got.(ir.Ptr)
	if !ok || !isPtr {
		return c.fail(ErrIR, in.Op(), "a zeroed slot with no address")
	}
	size := types.Sizeof(in.Result().Type().Object().Formal(), types.DefaultTarget64)
	off := int64(0)
	for ; off+8 <= size; off += 8 {
		c.b.I64.Store(c.b.I64.Const(0), c.fieldAddr(slot, off))
	}
	if off+4 <= size {
		c.b.I32.Store(c.b.I32.Const(0), c.fieldAddr(slot, off))
	}
	return nil
}

// classTableName is the name of the table an instance of a class is made
// with: an instance of a generic class has its own.
func classTableName(t types.Type, cl *types.Class) string {
	if gi, ok := t.(*types.GenericInstance); ok {
		return gi.String()
	}
	return cl.Name
}

// objectHeaderWords is the class instance header offset (metadata pointer + refcount).
const objectHeaderWords = stdlib.HeaderWords

// allocRef allocates a class instance through the runtime allocator and stores its vtable metadata.
func (c *fn) allocRef(in *sil.Inst) error {
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
	// Request instance storage size (header + stored properties).
	size, ok := types.InstanceSizeof(f, types.DefaultTarget64)
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), t.String()+": no layout for its stored properties")
	}
	if c.l.alloc == nil {
		c.l.alloc = c.l.out.ImportFunc(c.l.sym(stdlib.Alloc),
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

	// Store class dispatch table address into the instance metadata word.
	if cl, ok := f.Underlying().(*types.Class); ok {
		if g, ok := c.l.vtable[classTableName(f, cl)]; ok {
			c.b.Ptr.Store(c.b.Ptr.GetAddr(g), obj)
		}
	}
	c.def(res, obj)
	return nil
}

// classMethod resolves a class method implementation by indexing into the receiver's vtable.
func (c *fn) classMethod(in *sil.Inst) error {
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
	rows, ok := c.l.slots[classTableName(in.Args()[0].Type().Formal(), cl)]
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
	c.def(res, c.b.Ptr.Load(c.fieldAddr(table, int64(i+vtableFirstMethod)*8)))
	return nil
}

// classOf is the class a value's type is.
func classOf(t sil.Type) (*types.Class, bool) {
	if !t.IsValid() || t.Formal() == nil {
		return nil, false
	}
	cl, ok := t.Formal().Underlying().(*types.Class)
	return cl, ok
}

// makeEnum constructs an enum case value, lowering payloadless enums to their integer tag.
func (c *fn) makeEnum(in *sil.Inst) error {
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

// switchEnum lowers an enum branch into a jump table or tag dispatch.
func (c *fn) switchEnum(in *sil.Inst) error {
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
func enumOf(t sil.Type) (*types.Enum, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	e, ok := t.Formal().Underlying().(*types.Enum)
	return e, ok
}

// enumTag is a case's position among the ones its type declares, which
// is the value the case is represented by.
func enumTag(t sil.Type, member string) (int64, bool) {
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
func (c *fn) structElementAddr(in *sil.Inst) error {
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

// takeEnumDataAddr is a case's payload inside an enum's storage. Every
// payload starts where the enum does -- an optional's some, whatever
// its layout, and a payload enum's cases share the area before the tag
// -- so it is the enum's own address, typed as the payload.
func (c *fn) takeEnumDataAddr(in *sil.Inst) error {
	base, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	p, ok := base.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "the operand is not an address")
	}
	c.def(in.Result(), p)
	return nil
}

// refElementAddr is a stored property of a class instance. A class is
// a reference, so this is arithmetic on that reference and not a load.
func (c *fn) refElementAddr(in *sil.Inst) error {
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

func (c *fn) load(in *sil.Inst) error {
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
func (c *fn) loadScalar(in *sil.Inst, p ir.Ptr, r repr) (ir.Value, error) {
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
		return nil, c.fail(ErrType, opOf(in), r.reg.String())
	}
}

func (c *fn) store(in *sil.Inst) error {
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
func (c *fn) spread(v *sil.Value) ([]ir.Value, []leaf, bool) {
	ls, ok := leavesOf(v.Type())
	if !ok || len(ls) < 2 {
		return nil, nil, false
	}
	fields, ok := c.parts(v)
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
func (c *fn) storeScalar(in *sil.Inst, v ir.Value, p ir.Ptr, r repr) error {
	if r.narrow() {
		n, ok := v.(ir.I32)
		if !ok {
			return c.fail(ErrType, opOf(in), "a narrow value not in an i32 register")
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
		return c.fail(ErrType, opOf(in), "unstorable")
	}
	return nil
}

// refCount is a retain or a release. What they do belongs to the
// runtime, which runtime/ builds as a VIR module of its own; that
// they exist is all this package needs to know.
func (c *fn) refCount(in *sil.Inst, slot *ir.Callee, name string) error {
	// A closure context in the body's own storage is not counted: see
	// stackContext.
	if at, onStack := c.contexts[in.Args()[0]]; onStack {
		if rel, owns := c.releasers[in.Args()[0]]; owns && in.Op() == sil.StrongRelease {
			c.b.Call(rel, at)
		}
		return nil
	}
	// A String is counted through its own retain and release, which
	// read its tag. See string.go.
	if bridged(in.Args()[0].Type()) {
		return c.bridgeRefCount(in, in.Op() == sil.StrongRetain)
	}
	// A payload enum is counted as what its active case carries, and
	// an optional one the same way: its empty tag matches no case.
	if e, ok := payloadEnumOf(in.Args()[0].Type()); ok && enumCounted(e) {
		return c.enumRefCount(in, e, in.Op() == sil.StrongRetain)
	}
	if o, ok := optionalOfType(in.Args()[0].Type()); ok {
		if e, ok := enumOptional(o); ok && enumCounted(e) {
			return c.optionalEnumRefCount(in, e, in.Op() == sil.StrongRetain)
		}
	}
	// A struct is counted as the references it holds, each the way it
	// is counted on its own.
	if st, ok := structOf(in.Args()[0].Type()); ok && st != nil {
		return c.structRefCount(in, st, in.Op() == sil.StrongRetain)
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
		*slot = c.l.runtimeFunc(name, ir.NewSig().Param(ir.TypePtr))
	}
	c.b.Call(*slot, p)
	return nil
}

func (c *fn) apply(in *sil.Inst) error {
	if done, err := c.smallStringLiteral(in); done || err != nil {
		return err
	}
	if done, err := c.onceAccess(in); done || err != nil {
		return err
	}
	return c.call(in)
}

// call lowers an apply as the call it is.
func (c *fn) call(in *sil.Inst) error {
	callee, direct := c.refs[in.Args()[0]]
	var through, throughContext ir.Ptr
	if !direct {
		// A call through a value: a closure, or a function passed in.
		// The value is its code and its context, the context goes in the
		// self register, and the signature comes from the value's own
		// type, which is what keeps an indirect call well typed.
		code, context, err := c.calleeValue(in, in.Args()[0])
		if err != nil {
			return err
		}
		through, throughContext = code, context
	}
	// A result that comes back by address needs somewhere to come
	// back to, and its address goes first -- the same place the
	// signature declares it.
	var out ir.Ptr
	callResult := in.Result()
	var wantsSRet bool
	if callResult != nil {
		if _, yes := c.outResult(in, callResult); yes {
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
	// A call into a module swiftc built converts String and Array on
	// the way in and out. See swift.go.
	swift := direct && c.swiftCall(in.Args()[0])
	var params []sil.Param
	if swift {
		if f := c.l.module.Lookup(c.refNames[in.Args()[0]]); f != nil {
			params = f.Type().Params
		}
	}
	var afterCall []func()
	for i, a := range in.Args()[1:] {
		if swift {
			if isString, isArray, _ := swiftBridged(a.Type()); isString || isArray {
				var regs []ir.Value
				if isString {
					got, _, err := c.gatherArg(in, a)
					if err != nil {
						return err
					}
					regs = got
				} else {
					got, err := c.operand(in, a)
					if err != nil {
						return err
					}
					regs = []ir.Value{got}
				}
				conv := sil.ParamGuaranteed
				if i < len(params) {
					conv = params[i].Convention
				}
				bridged, release, err := c.bridgeArgToSwift(in, a, regs, conv)
				if err != nil {
					return err
				}
				if release != nil {
					afterCall = append(afterCall, release)
				}
				args = append(args, bridged...)
				continue
			}
		}
		// An argument of empty type is not passed, matching the
		// signature that does not declare a register for it.
		if empty(a.Type()) {
			continue
		}
		// One too wide for registers is passed as its address.
		if _, yes := indirect(a.Type()); yes {
			p, ok, err := c.wideAddress(in, a)
			if err != nil {
				return err
			}
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
		if _, isValue := in.Args()[0].Type().Formal().Underlying().(*types.Signature); isValue {
			args = append(args, throughContext)
		}
		res = c.b.CallInd(through, ft, args...)
	}
	for _, release := range afterCall {
		release()
	}

	// What came back by address is already where it belongs -- unless
	// it is a value that has a register, which is the `@out` case: a
	// generic function returns its result through the caller's
	// storage because the caller is the only one who knows how big it
	// is, and once it is back the caller knows exactly that. Array's
	// subscript getter hands an Int32 back through a four-byte slot,
	// and what the program asked for is the Int32.
	if wantsSRet {
		// Several registers' worth, handed back through storage because
		// the convention returns one: read back into the registers
		// everything downstream expects the value in.
		if n, split := c.l.splitResult(callResult.Type()); split {
			regs := make([]ir.Value, 0, n/8)
			for off := int64(0); off < n; off += 8 {
				regs = append(regs, c.b.I64.Load(c.fieldAddr(out, off)))
			}
			if swift {
				got, err := c.bridgeResultFromSwift(in, callResult, regs)
				if err != nil {
					return err
				}
				regs = got
			}
			return c.spreadInto(callResult, regs)
		}
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
		if swift {
			got, err := c.bridgeResultFromSwift(in, r, regs)
			if err != nil {
				return err
			}
			regs = got
		}
		return c.spreadInto(r, regs)
	}
	if swift {
		got, err := c.bridgeResultFromSwift(in, r, []ir.Value{res.Value(0)})
		if err != nil {
			return err
		}
		c.def(r, got[0])
		return nil
	}
	c.def(r, res.Value(0))
	return nil
}

func (c *fn) br(in *sil.Inst) error {
	dest, ok := c.blocks[in.Aux().Dest]
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "no such block")
	}
	c.b.Br(dest.To(c.args(in.Aux().Args)...))
	return nil
}

func (c *fn) condBr(in *sil.Inst) error {
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

func (c *fn) ret(in *sil.Inst) error {
	// An async function does not return: it tail calls the continuation
	// its context names, handing it the result as an argument. See
	// asyncReturn.
	if c.async {
		return c.asyncReturn(in)
	}
	if len(in.Args()) == 0 {
		c.finish()
		return nil
	}
	// A result that comes back by address is copied into the storage
	// the caller set aside, and the function returns nothing: the
	// value never was in registers and there is nothing to put in
	// them.
	if c.hasSRet {
		if err := c.retIndirect(in); err != nil {
			return err
		}
		c.finish()
		return nil
	}
	// A struct is returned in the words it is passed in.
	regs, wide, err := c.gather(in, in.Args()[0])
	if err != nil {
		return err
	}
	if wide {
		c.finish(regs...)
		return nil
	}
	if v, ok := c.value(in.Args()[0]); ok {
		c.finish(v)
		return nil
	}
	// A function whose result holds nothing returns nothing: SIL still
	// writes `return %0` with the empty tuple, and there is no register.
	if empty(in.Args()[0].Type()) {
		c.finish()
		return nil
	}
	return c.fail(ErrUnsupported, in.Op(), "a result held in memory")
}

// retIndirect writes a result that comes back by address into the
// storage the caller set aside. What returns afterwards is the caller's
// business: an ordinary function returns nothing, and an async one tail
// calls its continuation with nothing. See asyncReturn.
func (c *fn) retIndirect(in *sil.Inst) error {
	arg := in.Args()[0]
	// A result of several registers written through the caller's
	// storage, a word at a time. See splitResult.
	if _, split := c.l.splitResult(arg.Type()); split {
		if _, inMemory := c.mem[arg]; !inMemory {
			regs, wide, err := c.gather(in, arg)
			if err != nil {
				return err
			}
			if !wide {
				return c.fail(ErrUnsupported, in.Op(), "a split result that is not in words")
			}
			for i, r := range regs {
				word, ok := r.(ir.I64)
				if !ok {
					return c.fail(ErrType, in.Op(), "a word that is not an i64")
				}
				c.b.I64.Store(word, c.fieldAddr(c.sret, int64(i)*8))
			}
			return nil
		}
	}
	src, ok := c.mem[arg]
	if !ok {
		// Held as its scalars: written into the caller's storage.
		wrote, err := c.storeLeaves(in, arg, c.sret)
		if err != nil {
			return err
		}
		if !wrote {
			return c.fail(ErrUnsupported, in.Op(), "an indirect result that is not in memory")
		}
		return nil
	}
	size, _ := indirect(arg.Type())
	if n, split := c.l.splitResult(arg.Type()); split {
		size = n
	}
	c.b.MemCpy(c.sret, src, c.b.I64.Const(size))
	return nil
}

// errorSlot is the index of a signature's swifterror result, or -1.
func errorSlot(sig *ir.Sig) int {
	if sig == nil {
		return -1
	}
	for i, r := range sig.Rets() {
		for _, a := range r.Attrs {
			if a.IsSwiftError() {
				return i
			}
		}
	}
	return -1
}

// finish returns vals from the function, adding the error register's slot
// for a function that may fail: null, on a path that did not.
func (c *fn) finish(vals ...ir.Value) {
	sig := c.out.Signature()
	if i := errorSlot(sig); i >= 0 && len(vals) == len(sig.Rets())-1 {
		with := make([]ir.Value, 0, len(vals)+1)
		with = append(with, vals[:i]...)
		with = append(with, c.b.Ptr.Const())
		with = append(with, vals[i:]...)
		vals = with
	}
	c.b.Return(vals...)
}

// throwInst ends a path that failed: the error box in the error register,
// and a zero in each ordinary result, which a caller that sees the error
// does not read.
func (c *fn) throwInst(in *sil.Inst) error {
	sig := c.out.Signature()
	idx := errorSlot(sig)
	if idx < 0 && !c.async {
		return c.fail(ErrUnsupported, in.Op(), "a throw from a function that does not throw")
	}
	if len(in.Args()) != 1 {
		return c.fail(ErrUnsupported, in.Op(), "a throw of other than one error")
	}
	got, err := c.operand(in, in.Args()[0])
	if err != nil {
		return err
	}
	box, ok := got.(ir.Ptr)
	if !ok {
		return c.fail(ErrType, in.Op(), "an error that is not a reference")
	}
	if c.async {
		return c.asyncThrow(in, box)
	}
	vals := make([]ir.Value, 0, len(sig.Rets()))
	for i, r := range sig.Rets() {
		if i == idx {
			vals = append(vals, box)
			continue
		}
		switch r.Type {
		case ir.TypeI32:
			vals = append(vals, c.b.I32.Const(0))
		case ir.TypeI64:
			vals = append(vals, c.b.I64.Const(0))
		case ir.TypeF32:
			vals = append(vals, c.b.F32.Const(0))
		case ir.TypeF64:
			vals = append(vals, c.b.F64.Const(0))
		case ir.TypePtr:
			vals = append(vals, c.b.Ptr.Const())
		default:
			return c.fail(ErrUnsupported, in.Op(), "a throw from a function with a result of "+r.Type.String())
		}
	}
	c.b.Return(vals...)
	return nil
}

// asyncThrow is a throw from an async function: it goes forward like a
// return, with the box in the self register and nothing worth reading
// where the result would have been.
//
// swiftc's own failing path is
//
//	musttail call swifttailcc void %7(ptr swiftasync %0, i64 undef, ptr swiftself %5)
//
// -- the same continuation as the path that succeeded, told apart by
// whether the error is null.
func (c *fn) asyncThrow(in *sil.Inst, box ir.Ptr) error {
	if c.ctx.IsZero() {
		return c.fail(ErrUnsupported, in.Op(), "a throw from an async function with no context")
	}
	resume := c.b.Ptr.Load(c.b.Ptr.Add(c.ctx, c.b.I64.Const(8)))
	args := []ir.Value{c.ctx}
	// Whatever the result would have been, nothing reads it: the
	// continuation looks at the error first. Zero rather than undef,
	// because VIR has no undef and a register has to hold something.
	results, err := c.asyncResultRegs(in)
	if err != nil {
		return err
	}
	args = append(args, results...)
	args = append(args, box)
	c.b.TailCallInd(resume, c.l.continuationType(results, true), args...)
	return nil
}

// asyncResultRegs is a zero for each register this function's result
// comes back in, for the path that has no result to hand over.
func (c *fn) asyncResultRegs(in *sil.Inst) ([]ir.Value, error) {
	t := c.src.Type()
	if t == nil || c.hasSRet {
		return nil, nil
	}
	var out []ir.Value
	for _, res := range t.Results {
		rs, ok := frameRegs(res.Type)
		if !ok {
			return nil, c.fail(ErrType, in.Op(), whyNoRegister(res.Type))
		}
		for _, r := range rs {
			z, ok := zeroOf(c.b, r)
			if !ok {
				return nil, c.fail(ErrUnsupported, in.Op(),
					"a throw from a function with a result of "+r.String())
			}
			out = append(out, z)
		}
	}
	return out, nil
}

// zeroOf is a zero of a register type.
func zeroOf(b *ir.Block, r ir.RegType) (ir.Value, bool) {
	switch r {
	case ir.TypeI1:
		return b.I32.Ne(b.I32.Const(0), b.I32.Const(0)), true
	case ir.TypeI32:
		return b.I32.Const(0), true
	case ir.TypeI64:
		return b.I64.Const(0), true
	case ir.TypeF32:
		return b.F32.Const(0), true
	case ir.TypeF64:
		return b.F64.Const(0), true
	case ir.TypePtr:
		return b.Ptr.Const(), true
	}
	return nil, false
}

// thinToThick converts a function reference into a function value pointer.
func (c *fn) thinToThick(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	callee, ok := c.refs[in.Args()[0]]
	if !ok {
		return c.fail(ErrUnsupported, in.Op(), "a function value that is not a reference to a declaration")
	}
	// The code, and no context: a declared function captures nothing.
	//
	// An async one names the record beside its code instead, because
	// whoever calls through the value has to size the frame first and
	// only the record says how big it is. See asyncsplit.go.
	var code ir.Ptr
	if sig, ok := res.Type().Formal().Underlying().(*types.Signature); ok && sig.Async {
		name, named := c.refNames[in.Args()[0]]
		if !named {
			return c.fail(ErrUnsupported, in.Op(), "an async function value of an unnamed function")
		}
		code = c.b.Ptr.GetAddr(c.l.asyncRecordOf(name))
	} else {
		code = c.b.Ptr.GetAddr(callee)
	}
	c.multi[res] = []ir.Value{code, c.b.Ptr.Const()}
	return nil
}

// calleeValue is what an indirect call calls through: a function value's
// code and context, or -- for a method loaded out of a class's table or a
// witness table -- the one pointer that is, with no context.
func (c *fn) calleeValue(in *sil.Inst, v *sil.Value) (ir.Ptr, ir.Ptr, error) {
	if _, isValue := v.Type().Formal().Underlying().(*types.Signature); isValue {
		return c.functionValue(in, v)
	}
	got, err := c.operand(in, v)
	if err != nil {
		return ir.Ptr{}, ir.Ptr{}, err
	}
	p, ok := got.(ir.Ptr)
	if !ok {
		return ir.Ptr{}, ir.Ptr{}, c.fail(ErrType, in.Op(), "the callee is not a pointer")
	}
	return p, ir.Ptr{}, nil
}

// functionValue is a function value's two words: the code to call, and the
// context it is called with in the self register.
func (c *fn) functionValue(in *sil.Inst, v *sil.Value) (ir.Ptr, ir.Ptr, error) {
	regs, wide, err := c.gatherArg(in, v)
	if err != nil {
		return ir.Ptr{}, ir.Ptr{}, err
	}
	if !wide || len(regs) != 2 {
		return ir.Ptr{}, ir.Ptr{}, c.fail(ErrUnsupported, in.Op(), "a function value that is not its two words")
	}
	code, err := c.asPointer(in, regs[0])
	if err != nil {
		return ir.Ptr{}, ir.Ptr{}, err
	}
	context, err := c.asPointer(in, regs[1])
	if err != nil {
		return ir.Ptr{}, ir.Ptr{}, err
	}
	return code, context, nil
}

// calleeType returns the VIR function type for an indirect call target.
func (c *fn) calleeType(in *sil.Inst, v *sil.Value) (*ir.Type, error) {
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
	case *sil.FuncType:
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

// makeStructInMemory stores struct fields into an allocated memory slot in layout order.
func (c *fn) makeStructInMemory(in *sil.Inst, res *sil.Value) error {
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
		// A field of no bytes has nothing to write.
		if types.Sizeof(f.Type, types.DefaultTarget64) == 0 {
			continue
		}
		off, ok := types.Offsetof(st, f.Name, types.DefaultTarget64)
		if !ok {
			return c.fail(ErrType, in.Op(), "no offset for "+f.Name)
		}
		// A field that is itself in memory is copied there; one held as
		// several registers -- a String, a struct -- is stored a leaf at a
		// time, each at its own offset inside the field.
		if from, inMemory := c.mem[args[i]]; inMemory {
			size := types.Sizeof(f.Type, types.DefaultTarget64)
			c.b.MemCpy(c.fieldAddr(slot, off), from, c.b.I64.Const(size))
			continue
		}
		if parts, several := c.parts(args[i]); several {
			inner, ok := structOf(sil.Object(f.Type))
			if !ok {
				return c.fail(ErrType, in.Op(), whyNoRegister(sil.Object(f.Type)))
			}
			ls, ok := structLeaves(inner)
			if !ok || len(ls) != len(parts) {
				return c.fail(ErrType, in.Op(), whyNoRegister(sil.Object(f.Type)))
			}
			for k, l := range ls {
				r, ok := machineOf(l.typ)
				if !ok {
					return c.fail(ErrType, in.Op(), whyNoRegister(sil.Object(l.typ)))
				}
				if err := c.storeScalar(in, parts[k], c.fieldAddr(slot, off+l.offset), r); err != nil {
					return err
				}
			}
			continue
		}
		r, ok := machineOf(f.Type)
		if !ok {
			return c.fail(ErrType, in.Op(), whyNoRegister(sil.Object(f.Type)))
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
func (c *fn) loadFieldFrom(in *sil.Inst, base ir.Ptr, st *types.Struct, name string, ft types.Type) error {
	return c.loadFieldInto(in, in.Result(), base, st, name, ft)
}

// loadFieldInto is loadFieldFrom defining res, which may be one of
// several results.
func (c *fn) loadFieldInto(in *sil.Inst, res *sil.Value, base ir.Ptr, st *types.Struct, name string, ft types.Type) error {
	off, ok := types.Offsetof(st, name, types.DefaultTarget64)
	if !ok {
		return c.fail(ErrType, in.Op(), "no offset for "+name)
	}
	// A field of several registers is read a leaf at a time.
	if inner, isImage := structOf(sil.Object(ft)); isImage && inner != nil {
		if ls, ok := structLeaves(inner); ok && len(ls) > 1 {
			parts := make([]ir.Value, 0, len(ls))
			for _, l := range ls {
				r, ok := machineOf(l.typ)
				if !ok {
					return c.fail(ErrType, in.Op(), whyNoRegister(sil.Object(l.typ)))
				}
				v, err := c.loadScalar(in, c.fieldAddr(base, off+l.offset), r)
				if err != nil {
					return c.fail(ErrType, in.Op(), err.Error())
				}
				parts = append(parts, v)
			}
			c.multi[res] = parts
			return nil
		}
	}
	r, ok := machineOf(ft)
	if !ok {
		return c.fail(ErrType, in.Op(), whyNoRegister(sil.Object(ft)))
	}
	v, err := c.loadScalar(in, c.fieldAddr(base, off), r)
	if err != nil {
		return c.fail(ErrType, in.Op(), err.Error())
	}
	c.def(res, v)
	return nil
}

// Existential container layout offsets and sizes in bytes (3-word buffer, metadata, witness table).
const (
	existentialBuffer   = stdlib.ExistentialBuffer
	existentialMetadata = stdlib.ExistentialMetadata
	existentialWitness  = 4 * 8
	existentialSize     = 5 * 8
)

// initExistential populates an existential container's witness table and metadata,
// returning the address for the contained value (inline buffer or allocated box).
func (c *fn) initExistential(in *sil.Inst) error {
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
	// A value wider than the buffer goes in a box the runtime makes for
	// its type; the buffer holds the box, and the value is written into
	// it. See stdlib/ABI.md.
	if size, ok := storageBytes(concrete); ok && size > stdlib.ExistentialInline {
		alloc := c.l.runtimeFunc(stdlib.BoxAllocate, ir.NewSig().Param(ir.TypePtr).Ret(ir.TypePtr))
		box, ok := c.b.Call(alloc, record).Value(0).(ir.Ptr)
		if !ok {
			return c.fail(ErrType, in.Op(), "a box that is not a pointer")
		}
		c.b.Ptr.Store(box, c.fieldAddr(p, existentialBuffer))
		c.def(res, c.b.Ptr.Add(box, c.b.I64.Const(boxValueOffset(storageAlign(concrete)))))
		return nil
	}
	c.def(res, c.fieldAddr(p, existentialBuffer))
	return nil
}

// Value witness table layout offsets and flag bits.
const (
	valueWitnessOffset = stdlib.WitnessTableOffset
	valueWitnessFlags  = stdlib.WitnessFlags
	// valueWitnessDestroy is the witness function index that deinitializes a value.
	valueWitnessDestroy = stdlib.WitnessDestroy
	// vwIsNonInline indicates the value is stored out-of-line in a heap box.
	vwIsNonInline = stdlib.WitnessIsNonInline
)

// openExistential projects the address of a value contained within an existential container.
func (c *fn) openExistential(in *sil.Inst) error {
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
	mask := c.b.I64.And(flags, c.b.I64.Const(stdlib.WitnessAlignMask))
	off := c.b.I64.And(c.b.I64.Add(mask, c.b.I64.Const(stdlib.HeaderBytes)),
		c.b.I64.Not(mask))
	boxed := c.b.Ptr.Add(c.b.Ptr.Load(buf), off)

	boxes := c.b.I64.Ne(c.b.I64.And(flags, c.b.I64.Const(vwIsNonInline)),
		c.b.I64.Const(0))
	c.def(res, c.b.Ptr.Select(boxes, boxed, buf))
	return nil
}

// destroyExistential deinitializes an existential container by calling its destroy
// value witness (for inline values) or releasing the heap box (for boxed values).
func (c *fn) destroyExistential(in *sil.Inst) error {
	if len(in.Args()) == 0 {
		return c.fail(ErrUnsupported, in.Op(), "nothing to destroy")
	}
	_, isEx := existentialBytes(in.Args()[0].Type().Object())
	optionalEx := false
	if o, ok := optionalOf(in.Args()[0].Type().Object()); ok {
		switch o.Wrapped.Underlying().(type) {
		case *types.Existential, *types.Protocol:
			optionalEx = true
		}
	}
	if !isEx && !optionalEx {
		return c.fail(ErrUnsupported, in.Op(),
			"destroying something other than an existential by address")
	}
	p, err := c.addressOf(in, in.Args()[0])
	if err != nil {
		return err
	}
	buf := c.fieldAddr(p, existentialBuffer)
	meta := c.b.Ptr.Load(c.fieldAddr(p, existentialMetadata))
	// An optional existential that is empty -- its metadata word null --
	// holds nothing to end.
	if optionalEx {
		c.conts++
		held := c.out.Block("held" + itoa(c.conts))
		empty := c.out.Block("empty" + itoa(c.conts))
		c.b.BrIf(c.b.Ptr.Ne(meta, c.b.Ptr.Const()), held.To(), empty.To())
		c.b = held
		defer func(empty *ir.Block) {
			c.b.Br(empty.To())
			c.b = empty
		}(empty)
	}
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
	if c.l.release == nil {
		c.l.release = c.l.runtimeFunc(stdlib.Release, ir.NewSig().Param(ir.TypePtr))
	}
	c.b.Call(c.l.release, c.b.Ptr.Load(buf))
	c.b.Br(done.To())

	c.b = inline
	c.b.CallInd(c.b.Ptr.Load(c.fieldAddr(vwt, valueWitnessDestroy)),
		c.l.destroyWitnessType(), buf, meta)
	c.b.Br(done.To())

	c.b = done
	return nil
}

// copyAddr copies raw bytes between two addresses.
func (c *fn) copyAddr(in *sil.Inst) error {
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
	// An existential copied is a second owner of what it holds: the box a
	// wide value is in is retained, and an inline value is copied through
	// its witness, which counts whatever it holds.
	take := false
	for _, a := range in.Aux().Attrs {
		if a == "take" {
			take = true
		}
	}
	if _, isExistential := existentialBytes(in.Args()[0].Type().Object()); isExistential && !take {
		meta := c.b.Ptr.Load(c.fieldAddr(src, existentialMetadata))
		vwt := c.b.Ptr.Load(c.fieldAddr(meta, valueWitnessOffset))
		flags := c.b.I64.ZExtI32(c.b.I32.Load(c.fieldAddr(vwt, valueWitnessFlags)))
		boxes := c.b.I64.Ne(c.b.I64.And(flags, c.b.I64.Const(vwIsNonInline)), c.b.I64.Const(0))
		c.conts++
		n := itoa(c.conts)
		inBox := c.out.Block("copyboxed" + n)
		inline := c.out.Block("copyinline" + n)
		done := c.out.Block("copied" + n)
		c.b.BrIf(boxes, inBox.To(), inline.To())

		c.b = inBox
		if c.l.retain == nil {
			c.l.retain = c.l.runtimeFunc(stdlib.Retain, ir.NewSig().Param(ir.TypePtr))
		}
		c.b.Call(c.l.retain, c.b.Ptr.Load(c.fieldAddr(dst, existentialBuffer)))
		c.b.Br(done.To())

		c.b = inline
		c.b.CallInd(c.b.Ptr.Load(c.fieldAddr(vwt, stdlib.WitnessInitWithCopy)),
			c.l.copyWitnessType(), c.fieldAddr(dst, existentialBuffer), c.fieldAddr(src, existentialBuffer), meta)
		c.b.Br(done.To())

		c.b = done
	}
	return nil
}

// addressOf is where a value lives, whether it arrived in a register
// holding its address or was left in memory by whatever produced it.
func (c *fn) addressOf(in *sil.Inst, v *sil.Value) (ir.Ptr, error) {
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

// witnessMethod resolves a protocol witness method implementation from an existential witness table.
func (c *fn) witnessMethod(in *sil.Inst) error {
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

	// The first table entry is the conformance descriptor; requirement
	// rows follow at word offsets.
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
	// Save metadata and witness table passed as extra witness arguments.
	c.witnessExtra[res] = [2]ir.Value{
		c.b.Ptr.Load(c.fieldAddr(p, existentialMetadata)),
		table,
	}
	return nil
}

// witnessRow is a row's place in a protocol's table.
func (c *fn) witnessRow(in *sil.Inst, proto, row string) (int, error) {
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
func protocolsOf(t sil.Type) ([]string, bool) {
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
func typeNameOfType(t sil.Type) string {
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

// metatype lowers a metatype reference, emitting a metadata accessor call or record lookup when required.
func (c *fn) metatype(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	mt, ok := in.Aux().Type.Formal().(*sil.MetatypeType)
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
func optionalOf(t sil.Type) (*types.Optional, bool) {
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return nil, false
	}
	o, ok := t.Formal().Underlying().(*types.Optional)
	return o, ok
}

// makeOptional constructs an Optional value (.some or .none) using either an extra tag byte or null representation.
func (c *fn) makeOptional(in *sil.Inst, res *sil.Value, o *types.Optional) error {
	some := in.Aux().Member == optionalSomeCase
	image, tagged := optionalImage(o)
	if emptyOptional(o) {
		tag := int64(0)
		if !some {
			tag = 1
		}
		c.def(res, c.b.I32.Const(tag))
		return nil
	}
	if e, ok := tagOptional(o); ok {
		if some {
			if len(in.Args()) != 1 {
				return c.fail(ErrUnsupported, in.Op(), "a case with no value to carry")
			}
			c.forward(res, in.Args()[0])
			return nil
		}
		r, ok := machineOf(e)
		if !ok {
			return c.fail(ErrType, in.Op(), res.Type().String())
		}
		none := int64(len(e.Cases))
		if r.reg == ir.TypeI64 {
			c.def(res, c.b.I64.Const(none))
		} else {
			c.def(res, c.b.I32.Const(none))
		}
		return nil
	}
	if boolOptional(o) {
		if !some {
			c.def(res, c.b.I32.Const(2))
			return nil
		}
		if len(in.Args()) != 1 {
			return c.fail(ErrUnsupported, in.Op(), "a case with no value to carry")
		}
		got, err := c.operand(in, in.Args()[0])
		if err != nil {
			return err
		}
		bit, ok := got.(ir.I1)
		if !ok {
			return c.fail(ErrType, in.Op(), "a Bool that is not a bit")
		}
		c.def(res, c.b.I32.ZExtI1(bit))
		return nil
	}
	if isFuncOptional(o) {
		if !some {
			c.multi[res] = []ir.Value{c.b.Ptr.Const(), c.b.Ptr.Const()}
			return nil
		}
		if len(in.Args()) != 1 {
			return c.fail(ErrUnsupported, in.Op(), "a case with no value to carry")
		}
		c.forward(res, in.Args()[0])
		return nil
	}
	if isStringOptional(o) {
		if !some {
			c.multi[res] = []ir.Value{c.b.I64.Const(0), c.b.I64.Const(0)}
			return nil
		}
		if len(in.Args()) != 1 {
			return c.fail(ErrUnsupported, in.Op(), "a case with no value to carry")
		}
		c.forward(res, in.Args()[0])
		return nil
	}
	// A struct that spares a representation: its own words, or all zeros.
	if st, _, spare := spareStructOptional(o); spare {
		if some {
			if len(in.Args()) != 1 {
				return c.fail(ErrUnsupported, in.Op(), "a case with no value to carry")
			}
			a := in.Args()[0]
			// A payload held in memory arrives as the words the optional
			// is held in.
			if from, inMem := c.mem[a]; inMem {
				if _, resInMem := c.mem[res]; !resInMem {
					leaves, ok := structLeaves(st)
					if !ok {
						return c.fail(ErrUnsupported, in.Op(), "a payload whose layout this cannot take apart")
					}
					parts := make([]ir.Value, 0, len(leaves))
					for _, l := range leaves {
						r, ok := machineOf(l.typ)
						if !ok {
							return c.fail(ErrType, in.Op(), l.typ.String())
						}
						w, err := c.loadScalar(in, c.fieldAddr(from, l.offset), r)
						if err != nil {
							return err
						}
						parts = append(parts, w)
					}
					c.multi[res] = parts
					return nil
				}
			}
			c.forward(res, a)
			return nil
		}
		zeros, err := c.zeroLeaves(in, o.Wrapped)
		if err != nil {
			return err
		}
		if len(zeros) == 1 {
			c.def(res, zeros[0])
			return nil
		}
		c.multi[res] = zeros
		return nil
	}
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
		if inner, ok := c.parts(a); ok {
			parts = append(parts, inner...)
		} else if from, inMemory := c.mem[a]; inMemory {
			// A payload built where it is kept, read back out as the
			// scalars the optional carries.
			ls, ok := leavesOf(a.Type())
			if !ok {
				return c.fail(ErrUnsupported, in.Op(), "a payload in memory whose layout this cannot take apart")
			}
			for _, l := range ls {
				r, ok := machineOf(l.typ)
				if !ok {
					return c.fail(ErrType, in.Op(), l.typ.String())
				}
				v, err := c.loadScalar(in, c.fieldAddr(from, l.offset), r)
				if err != nil {
					return err
				}
				parts = append(parts, v)
			}
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
func (c *fn) zeroLeaves(in *sil.Inst, t types.Type) ([]ir.Value, error) {
	// A payload enum is its words.
	if e, ok := t.Underlying().(*types.Enum); ok && hasPayload(e) {
		image, ok := enumImage(e)
		if !ok {
			return nil, c.fail(ErrUnsupported, in.Op(), e.Name)
		}
		out := make([]ir.Value, len(image.Fields))
		for i := range out {
			out[i] = c.b.I64.Const(0)
		}
		return out, nil
	}
	// Anything laid out as a struct is -- a struct, a tuple, a String,
	// a function, an optional with an image of its own -- is a zero
	// for each of its leaves.
	if st, ok := structOf(sil.Object(t)); ok {
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
func (c *fn) zeroOf(in *sil.Inst, t types.Type) (ir.Value, error) {
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

// switchOptional branches on an optional's presence (.some vs .none) based on its tag byte or pointer nullness.
func (c *fn) switchOptional(in *sil.Inst, o *types.Optional) error {
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

	// An optional of a value of no bytes is its tag: zero for a value.
	if emptyOptional(o) {
		got, err := c.operand(in, v)
		if err != nil {
			return err
		}
		tag, ok := got.(ir.I32)
		if !ok {
			return c.fail(ErrType, in.Op(), "an optional's tag that is not a register")
		}
		var args []ir.Value
		if _, ok := machineOf(o.Wrapped); ok {
			args = []ir.Value{c.b.I32.Const(0)}
		}
		isSome := c.b.I32.Eq(c.b.I32.And(tag, c.b.I32.Const(255)), c.b.I32.Const(0))
		c.b.BrIf(isSome, some.To(args...), none.To())
		return nil
	}
	// An optional of a plain enum is its tag, and nil the tag one past
	// the last case. See tagOptional.
	if e, ok := tagOptional(o); ok {
		got, err := c.operand(in, v)
		if err != nil {
			return err
		}
		nilTag := int64(len(e.Cases))
		var isSome ir.I1
		switch tag := got.(type) {
		case ir.I32:
			isSome = c.b.I32.Ne(tag, c.b.I32.Const(nilTag))
		case ir.I64:
			isSome = c.b.I64.Ne(tag, c.b.I64.Const(nilTag))
		default:
			return c.fail(ErrType, in.Op(), "an optional enum whose tag is not an integer register")
		}
		c.b.BrIf(isSome, some.To(got), none.To())
		return nil
	}
	// An optional Bool's empty case is the byte 2; the payload is the byte.
	if boolOptional(o) {
		got, err := c.operand(in, v)
		if err != nil {
			return err
		}
		tag, ok := got.(ir.I32)
		if !ok {
			return c.fail(ErrType, in.Op(), "an optional Bool that is not a register")
		}
		byte := c.b.I32.And(tag, c.b.I32.Const(255))
		isSome := c.b.I32.Ne(byte, c.b.I32.Const(2))
		c.b.BrIf(isSome, some.To(c.b.I32.Ne(byte, c.b.I32.Const(0))), none.To())
		return nil
	}
	// An optional function's empty case is a null code pointer; the
	// payload is both words.
	if isFuncOptional(o) {
		var words []ir.Value
		if parts, ok := c.parts(v); ok && len(parts) == 2 {
			words = parts
		} else if from, ok := c.mem[v]; ok {
			words = []ir.Value{c.b.Ptr.Load(from), c.b.Ptr.Load(c.fieldAddr(from, 8))}
		} else {
			return c.fail(ErrUnsupported, in.Op(), "an optional function that is neither in its two words nor in memory")
		}
		code, err := c.asPointer(in, words[0])
		if err != nil {
			return err
		}
		isSome := c.b.Ptr.Ne(code, c.b.Ptr.Const())
		c.b.BrIf(isSome, some.To(words...), none.To())
		return nil
	}
	// A struct that spares a representation: nil is its never-zero word
	// zero, and the payload is every word.
	if st, at, spare := spareStructOptional(o); spare {
		leaves, ok := structLeaves(st)
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "an optional of "+st.Name+" whose layout this cannot take apart")
		}
		var words []ir.Value
		if parts, ok := c.parts(v); ok && len(parts) == len(leaves) {
			words = parts
		} else if _, inMem := c.mem[v]; len(leaves) == 1 && !inMem {
			// One word, held in its own register.
			got, err := c.operand(in, v)
			if err != nil {
				return err
			}
			words = []ir.Value{got}
		} else if from, ok := c.mem[v]; ok {
			for _, l := range leaves {
				r, ok := machineOf(l.typ)
				if !ok {
					return c.fail(ErrType, in.Op(), l.typ.String())
				}
				w, err := c.loadScalar(in, c.fieldAddr(from, l.offset), r)
				if err != nil {
					return err
				}
				words = append(words, w)
			}
		} else {
			return c.fail(ErrUnsupported, in.Op(), "an optional struct neither in its words nor in memory")
		}
		var key ir.Value
		for i, l := range leaves {
			if l.offset == at {
				key = words[i]
			}
		}
		if key == nil {
			return c.fail(ErrUnsupported, in.Op(), "an optional struct whose never-zero word is not a leaf")
		}
		var isSome ir.I1
		switch k := key.(type) {
		case ir.Ptr:
			isSome = c.b.Ptr.Ne(k, c.b.Ptr.Const())
		case ir.I64:
			isSome = c.b.I64.Ne(k, c.b.I64.Const(0))
		default:
			return c.fail(ErrType, in.Op(), "an optional struct's never-zero word is not a word")
		}
		c.b.BrIf(isSome, some.To(words...), none.To())
		return nil
	}
	// A String's empty case is its object word zero; the payload is
	// both words.
	if isStringOptional(o) {
		var words []ir.Value
		if parts, ok := c.parts(v); ok && len(parts) == 2 {
			words = parts
		} else if from, ok := c.mem[v]; ok {
			words = []ir.Value{c.b.I64.Load(from), c.b.I64.Load(c.fieldAddr(from, 8))}
		} else {
			return c.fail(ErrUnsupported, in.Op(), "a String optional that is neither in its two words nor in memory")
		}
		object, ok := c.toWord(words[1], 64)
		if !ok {
			return c.fail(ErrType, in.Op(), "a String's second word that is not a word")
		}
		isSome := c.b.I64.Ne(object, c.b.I64.Const(0))
		c.b.BrIf(isSome, some.To(words...), none.To())
		return nil
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

	parts, ok := c.parts(v)
	if !ok {
		// Too wide for registers, it is in memory: its leaves are read
		// out of it, the tag last.
		from, inMem := c.mem[v]
		if !inMem {
			return c.fail(ErrUnsupported, in.Op(), "an optional this function did not take apart")
		}
		ls, ok := leavesOf(v.Type())
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "an optional in memory whose layout this cannot take apart")
		}
		for _, l := range ls {
			r, ok := machineOf(l.typ)
			if !ok {
				return c.fail(ErrType, in.Op(), l.typ.String())
			}
			w, err := c.loadScalar(in, c.fieldAddr(from, l.offset), r)
			if err != nil {
				return err
			}
			parts = append(parts, w)
		}
	}
	if len(parts) < 2 {
		return c.fail(ErrUnsupported, in.Op(), "an optional with no tag to switch on")
	}
	// The payload is everything before the tag, which is last. A
	// block argument may be several registers -- the arm declares one
	// parameter per leaf, the way a struct parameter arrives as its
	// words -- so the whole payload is passed on rather than its
	// first register.
	payload := parts[:len(parts)-1]
	// A payload enum's last word runs past its size to where the tag
	// is, and the tag's byte is not the enum's.
	if e, ok := enumOptional(o); ok && len(payload) > 0 {
		if rem := types.Sizeof(e, types.DefaultTarget64) % 8; rem != 0 {
			if last, ok := payload[len(payload)-1].(ir.I64); ok {
				masked := append([]ir.Value(nil), payload...)
				masked[len(masked)-1] = c.b.I64.And(last, c.b.I64.Const(int64(uint64(1)<<(uint(rem)*8)-1)))
				payload = masked
			}
		}
	}
	tag, ok := c.toWord(parts[len(parts)-1], 8)
	if !ok {
		return c.fail(ErrType, in.Op(), "a tag this cannot read")
	}
	isSome := c.b.I64.Eq(tag, c.b.I64.Const(0))
	c.b.BrIf(isSome, some.To(payload...), none.To())
	return nil
}

// optionalNoneCase is what gen calls the case that carries nothing.
const optionalNoneCase = "Optional.none"

// extractElement extracts an element from a multi-register tuple or aggregate tuple value.
func (c *fn) extractElement(in *sil.Inst) error {
	if res := in.Result(); res != nil && holdsNothing(res.Type()) {
		return c.defNothing(in, res)
	}
	v := in.Args()[0]
	i := int(in.Aux().Int)
	if i < 0 {
		return c.fail(ErrUnsupported, in.Op(), "element out of range")
	}
	parts, multi := c.parts(v)
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
func (c *fn) destructureTuple(in *sil.Inst) error {
	v := in.Args()[0]
	// A tuple held in memory -- too wide for registers -- gives each
	// element out of its storage, as a field is read out of a struct.
	if base, inMem := c.mem[v]; inMem {
		st, ok := structOf(v.Type())
		if !ok || st == nil {
			return c.fail(ErrType, in.Op(), v.Type().String())
		}
		for i, res := range in.Results() {
			if res == nil {
				continue
			}
			if i >= len(st.Fields) || st.Fields[i] == nil {
				return c.fail(ErrUnsupported, in.Op(), "a tuple with no element "+itoa(i))
			}
			if holdsNothing(res.Type()) {
				if err := c.defNothing(in, res); err != nil {
					return err
				}
				continue
			}
			if err := c.loadFieldInto(in, res, base, st, st.Fields[i].Name, st.Fields[i].Type); err != nil {
				return err
			}
		}
		return nil
	}
	parts, multi := c.parts(v)
	for i, res := range in.Results() {
		if res == nil {
			continue
		}
		lo, hi, ok := fieldLeaves(v.Type(), itoa(i))
		if !ok {
			// A labelled tuple's elements go by their labels.
			if st, found := structOf(v.Type()); found && i < len(st.Fields) && st.Fields[i] != nil {
				lo, hi, ok = fieldLeaves(v.Type(), st.Fields[i].Name)
			}
		}
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "a tuple with no element "+itoa(i))
		}
		if lo == hi {
			// An element of no bytes has no leaf and no register in the
			// tuple; what it is is its only value.
			if err := c.defNothing(in, res); err != nil {
				return err
			}
			continue
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

// tryApply lowers a throwing call terminator, branching to normal or error blocks based on the error register.
func (c *fn) tryApply(in *sil.Inst) error {
	var normal, failed *ir.Block
	var failedArgs int
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
			failedArgs = len(k.Dest.Args())
		}
	}
	if normal == nil || failed == nil {
		return c.fail(ErrUnsupported, in.Op(), "a try with fewer than both edges")
	}

	callee, direct := c.refs[in.Args()[0]]
	var through, throughContext ir.Ptr
	if !direct {
		// A try through a value -- a closure, or a function passed in --
		// is an indirect call typed by the value, as an apply of one is.
		code, context, err := c.calleeValue(in, in.Args()[0])
		if err != nil {
			return err
		}
		through, throughContext = code, context
	}
	// A value too wide for registers comes back through storage set
	// aside for it, whose address goes first, as it does for an apply.
	var out ir.Ptr
	var outArg *sil.Value
	for _, k := range in.Aux().Cases {
		if k.Member != "normal" || len(k.Dest.Args()) != 1 {
			continue
		}
		if _, wide := indirect(k.Dest.Args()[0].Type()); wide {
			slot, ok := c.wide[k.Dest.Args()[0]]
			if !ok {
				return c.fail(ErrIR, in.Op(), "no slot reserved for a wide result")
			}
			out, outArg = slot, k.Dest.Args()[0]
		}
	}
	var args []ir.Value
	if outArg != nil {
		args = append(args, out)
	}
	for _, a := range in.Args()[1:] {
		if empty(a.Type()) {
			continue
		}
		// One too wide for registers is passed as its address, as an
		// apply passes it.
		if _, yes := indirect(a.Type()); yes {
			p, ok, err := c.wideAddress(in, a)
			if err != nil {
				return err
			}
			if !ok {
				return c.fail(ErrUnsupported, in.Op(), "a struct passed by address that is not in memory")
			}
			args = append(args, p)
			continue
		}
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

	var res ir.Results
	if direct {
		res = c.b.Call(callee, args...)
	} else {
		ft, err := c.calleeType(in, in.Args()[0])
		if err != nil {
			return err
		}
		if _, isValue := in.Args()[0].Type().Formal().Underlying().(*types.Signature); isValue {
			args = append(args, throughContext)
		}
		res = c.b.CallInd(through, ft, args...)
	}
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
	// A struct comes back in the words it is passed in, and the normal
	// edge declares its fields: the words are taken apart, as an apply's
	// result is. See spreadInto.
	if outArg == nil && len(carried) > 0 {
		for _, k := range in.Aux().Cases {
			if k.Member != "normal" || len(k.Dest.Args()) != 1 {
				continue
			}
			st, ok := structOf(k.Dest.Args()[0].Type())
			if !ok {
				continue
			}
			ls, lok := structLeaves(st)
			words, wok := structWords(st)
			if !lok || !wok || len(words) != len(carried) {
				continue
			}
			if fields, ok := c.unpackStruct(carried, words, ls); ok {
				carried = fields
			}
		}
	}
	// The normal edge takes the wide value as its scalars, read back out
	// of the storage the callee wrote it into.
	if outArg != nil {
		ls, ok := leavesOf(outArg.Type())
		if !ok {
			return c.fail(ErrUnsupported, in.Op(), "a wide result whose layout this cannot take apart")
		}
		for _, l := range ls {
			r, ok := machineOf(l.typ)
			if !ok {
				return c.fail(ErrType, in.Op(), l.typ.String())
			}
			v, err := c.loadScalar(in, c.fieldAddr(out, l.offset), r)
			if err != nil {
				return err
			}
			carried = append(carried, v)
		}
	}
	threw := c.b.Ptr.Ne(errValue, c.b.Ptr.Const())
	// The error edge carries the box to a block that takes it -- a catch,
	// or a rethrow -- and to one that ignores it, `try?`, nothing.
	var errArgs []ir.Value
	if failedArgs == 1 {
		errArgs = []ir.Value{errValue}
	}
	c.b.BrIf(threw, failed.To(errArgs...), normal.To(carried...))
	return nil
}

// boxValueOffset is where in a box a value of this alignment is: past
// the header, rounded up to the alignment, which is where Swift's boxes
// put it too.
func boxValueOffset(align int64) int64 {
	if align < 1 {
		align = 1
	}
	return (stdlib.HeaderBytes + align - 1) &^ (align - 1)
}

// globalAddr is the address of a module-level variable's storage: zeroed
// writable data this module defines, which the variable's addressor
// initializes on first use.
func (c *fn) globalAddr(in *sil.Inst) error {
	res := in.Result()
	if res == nil {
		return nil
	}
	g, err := c.globalStorage(in.Aux().Name, in.Aux().Type)
	if err != nil {
		return c.fail(ErrType, in.Op(), err.Error())
	}
	c.def(res, c.b.Ptr.GetAddr(g))
	return nil
}

// globalStorage is the storage of the module-level variable name, of type
// t: zeroed, writable, made the first time it is asked for.
func (c *fn) globalStorage(silName string, t sil.Type) (*ir.Global, error) {
	f := t.Formal()
	if f == nil {
		return nil, fmt.Errorf("a global with no type")
	}
	size := types.Sizeof(f, types.DefaultTarget64)
	align := types.Alignof(f, types.DefaultTarget64)
	if size < 0 || align <= 0 {
		return nil, fmt.Errorf("%s", t.String())
	}
	if size == 0 {
		size = 1
	}
	name := c.l.sym(silName)
	g, ok := c.l.globalStore[name]
	if !ok {
		g = c.l.out.Global(name, ir.RW, ir.Array(uint64(size), ir.StoreI8.FType())).
			Init(ir.ZeroInit).Align(uint64(align))
		if c.l.globalStore == nil {
			c.l.globalStore = map[string]*ir.Global{}
		}
		c.l.globalStore[name] = g
	}
	return g, nil
}

// onceAccessor is, for the addressor of a module-level variable of this
// module that is initialized on first use, the variable and its once
// flag: the function reads the flag, runs the initializer if it is clear,
// and answers the variable's address.
type onceAccessor struct {
	value, once *sil.Global
}

// accessorOf recognizes such an addressor by what it touches.
func (l *lowerer) accessorOf(name string) (onceAccessor, bool) {
	if l.onces == nil {
		l.onces = map[string]*onceAccessor{}
	}
	if a, seen := l.onces[name]; seen {
		if a == nil {
			return onceAccessor{}, false
		}
		return *a, true
	}
	l.onces[name] = nil
	f := l.module.Lookup(name)
	if f == nil || f.IsDeclaration() || !strings.HasSuffix(name, "vau") {
		return onceAccessor{}, false
	}
	globals := map[string]*sil.Global{}
	for _, g := range l.module.Globals() {
		globals[g.Name()] = g
	}
	var a onceAccessor
	for _, b := range f.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() != sil.GlobalAddr {
				continue
			}
			n := in.Aux().Name
			if strings.HasSuffix(n, "_once") {
				a.once = globals[n]
				a.value = globals[strings.TrimSuffix(n, "_once")]
			}
		}
	}
	if a.once == nil || a.value == nil {
		return onceAccessor{}, false
	}
	l.onces[name] = &a
	return a, true
}

// onceAccess lowers a call of such an addressor the way swiftc's callers
// check swift_once's token: the flag is read in place, and only while it
// is clear -- the first time -- is the addressor called. Every later read
// of a global let is a load and a branch rather than a call.
func (c *fn) onceAccess(in *sil.Inst) (bool, error) {
	res := in.Result()
	if res == nil || len(in.Args()) != 1 {
		return false, nil
	}
	a, ok := c.l.accessorOf(c.refNames[in.Args()[0]])
	if !ok {
		return false, nil
	}
	flag, err := c.globalStorage(a.once.Name(), a.once.Type())
	if err != nil {
		return false, nil
	}
	value, err := c.globalStorage(a.value.Name(), a.value.Type())
	if err != nil {
		return false, nil
	}
	c.conts++
	n := itoa(c.conts)
	ready := c.out.Block("once_ready" + n)
	first := c.out.Block("once_first" + n)
	join := c.out.Block("once_join" + n)
	at := join.ParamPtr("once_addr" + n)
	set := c.b.I32.Ne(c.b.I32.ULoad8(c.b.Ptr.GetAddr(flag)), c.b.I32.Const(0))
	c.b.BrIf(set, ready.To(), first.To())
	ready.Br(join.To(ready.Ptr.GetAddr(value)))
	c.b = first
	if err := c.call(in); err != nil {
		return true, err
	}
	got, ok := c.value(res)
	if !ok {
		return true, c.fail(ErrIR, in.Op(), "an addressor with no address")
	}
	c.b.Br(join.To(got))
	c.b = join
	c.def(res, at)
	return true, nil
}

// wideAddress is where a value too wide for registers is: the memory it
// is held in, or -- held as its scalars -- the storage set aside for it,
// with the scalars written there first.
func (c *fn) wideAddress(in *sil.Inst, v *sil.Value) (ir.Ptr, bool, error) {
	if p, ok := c.mem[v]; ok {
		return p, true, nil
	}
	slot, ok := c.wide[v]
	if !ok {
		slot, ok = c.spill[v]
	}
	if !ok {
		return ir.Ptr{}, false, nil
	}
	wrote, err := c.storeLeaves(in, v, slot)
	if err != nil || !wrote {
		return ir.Ptr{}, false, err
	}
	// The scalars are in the slot from here on -- on this path. A use
	// in another block may be reached without passing this store, so
	// the slot is remembered as the value's home only when the store
	// is in the block that defines the value, which every use is
	// reached through; elsewhere the next use writes the slot again.
	if v.Block() == in.Block() {
		c.mem[v] = slot
	}
	return slot, true, nil
}

// storeLeaves writes a value held as its scalars into memory at dst, each
// at its offset, and reports whether it was held that way.
func (c *fn) storeLeaves(in *sil.Inst, v *sil.Value, dst ir.Ptr) (bool, error) {
	parts, ok := c.parts(v)
	if !ok {
		return false, nil
	}
	ls, ok := leavesOf(v.Type())
	if !ok || len(ls) != len(parts) {
		return false, c.fail(ErrUnsupported, opOf(in), "a value whose scalars do not match its layout")
	}
	for i, l := range ls {
		r, ok := machineOf(l.typ)
		if !ok {
			return false, c.fail(ErrType, opOf(in), l.typ.String())
		}
		if err := c.storeScalar(in, parts[i], c.fieldAddr(dst, l.offset), r); err != nil {
			return false, err
		}
	}
	return true, nil
}
