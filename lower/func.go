package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/vil"
)

// A fn is one function being translated: the two sides of it, and the
// correspondence between them.
type fn struct {
	l   *lowerer
	src *vil.Func
	out *ir.Func

	blocks map[*vil.Block]*ir.Block
	values map[*vil.Value]ir.Value
	// multi holds the results of an instruction that produced more than
	// one register -- an overflowing builtin -- for tuple_extract to
	// take apart again.
	multi map[*vil.Value][]ir.Value
	// refs holds what a function_ref named, since a thin function
	// reference is a symbol rather than a register until something
	// other than a call asks for its address.
	refs map[*vil.Value]ir.Callee
	// mem holds a value that lives at an address rather than in
	// registers: a struct too wide to be passed in them, which Swift
	// passes and returns by address. See indirect() in abi.go.
	mem map[*vil.Value]ir.Ptr
	// wide is the slot reserved for a value that lives in memory,
	// allocated in the entry block before any block is walked.
	wide map[*vil.Value]ir.Ptr
	// sret is the storage a caller set aside for this function's
	// result, when the result is one that comes back by address. It is
	// the function's first parameter and is not one of the VIL
	// function's own.
	sret    ir.Ptr
	hasSRet bool
	// witnessExtra is what a witness call passes beyond the arguments
	// the source wrote: the metadata for the type that is answering
	// and the table the call came through, read out of the
	// existential the witness was looked up in. See witnessMethod.
	witnessExtra map[*vil.Value][2]ir.Value

	b     *ir.Block // the block being filled
	trap  *ir.Block // where a failed cond_fail goes, made on demand
	conts int       // how many times a block has been split around a trap
}

func (l *lowerer) define(f *vil.Func) error {
	c := &fn{
		l:      l,
		src:    f,
		out:    l.defs[f.Name()],
		blocks: make(map[*vil.Block]*ir.Block),
		values: make(map[*vil.Value]ir.Value),
		multi:  make(map[*vil.Value][]ir.Value),
		refs:   make(map[*vil.Value]ir.Callee),
		mem:    make(map[*vil.Value]ir.Ptr),
		wide:   make(map[*vil.Value]ir.Ptr),

		witnessExtra: make(map[*vil.Value][2]ir.Value),
	}

	// Every block and every block parameter is created before any
	// instruction is emitted. A VIR block freezes its parameter list at
	// its first instruction, and a branch backwards must find the
	// parameters already there.
	for _, b := range f.Blocks() {
		if err := c.openBlock(b); err != nil {
			return err
		}
	}
	// The trap block is made here, after every block the source had,
	// so that it reads at the end of the function rather than wherever
	// the first overflow check happened to be.
	if needsTrap(f) {
		c.trapBlock()
	}
	if err := c.allocSlots(); err != nil {
		return err
	}
	for _, b := range f.Blocks() {
		c.b = c.blocks[b]
		for _, in := range b.Insts() {
			if err := c.inst(in); err != nil {
				return err
			}
		}
	}
	// The IR builder is sticky: the first failure inside it is
	// recorded and every call after it is a no-op. Asking here is what
	// keeps that failure attached to the function that caused it —
	// without it the next function's first instruction is the one that
	// appears to be wrong, which is a long way from the truth and a
	// long way from the line to fix.
	if err := c.l.out.Err(); err != nil {
		return &Error{Err: ErrIR, Func: f.SourceName(), What: err.Error()}
	}
	return nil
}

// allocSlots reserves the frame storage for every alloc_stack in the
// function, in the entry block, before any of them is reached.
//
// VIR admits a frame allocation in the entry block only — §19.6, and
// ir's builder refuses one anywhere else — while SIL writes
// alloc_stack wherever the variable was declared, which for a `var`
// inside a loop is a block that runs many times. The two are not in
// conflict: a slot is frame storage, the frame lasts the call, and a
// scalar local declared in a loop is one slot written afresh on each
// pass rather than a new slot each time. VIL already stores the
// initializer where the declaration was, so reusing the slot is what
// the program says.
//
// What this does not do is make the slot's lifetime shorter than the
// call. dealloc_stack becomes nothing, and two variables in disjoint
// scopes get two slots where one would have done. That is a frame
// larger than it needs to be and never a wrong program, and shrinking
// it is a job for a pass that knows the live ranges.
func (c *fn) allocSlots() error {
	entry := c.out.Entry()
	saved := c.b
	c.b = entry
	defer func() { c.b = saved }()

	for _, b := range c.src.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() == vil.AllocStack {
				if err := c.allocStack(in); err != nil {
					return err
				}
				continue
			}
			// A value too wide for registers lives in a slot, and a
			// slot is reserved here for the same reason an
			// alloc_stack's is: VIR admits an allocation in the entry
			// block only. Where it is built and where it is read are
			// wherever the program put them; where it lives is here.
			res := in.Result()
			if res == nil {
				continue
			}
			if _, ok := c.wide[res]; ok {
				continue
			}
			switch in.Op() {
			case vil.Struct, vil.Apply, vil.TryApply:
			default:
				continue
			}
			size, yes := outResult(in, res)
			if !yes {
				continue
			}
			c.wide[res] = c.b.Ptr.Alloc(uint64(size),
				uint64(storageAlign(res.Type())))
		}
	}
	return nil
}

// spreadInto records a struct that arrived in words as the fields the
// body will read.
func (c *fn) spreadInto(v *vil.Value, regs []ir.Value) error {
	st, ok := structOf(v.Type())
	if !ok {
		return c.fail(ErrType, vil.Op(""), v.Type().String())
	}
	ls, ok := structLeaves(st)
	if !ok {
		return c.fail(ErrUnsupported, vil.Op(""), whyNoRegister(v.Type()))
	}
	words, ok := structWords(st)
	if !ok {
		return c.fail(ErrUnsupported, vil.Op(""), whyNoRegister(v.Type()))
	}
	fields, ok := c.unpackStruct(regs, words, ls)
	if !ok {
		return c.fail(ErrUnsupported, vil.Op(""), whyNoRegister(v.Type()))
	}
	if len(fields) == 1 {
		c.def(v, fields[0])
		return nil
	}
	c.multi[v] = fields
	return nil
}

// gather is the other direction: the words a struct is passed in,
// built from the registers its fields are held in.
func (c *fn) gather(in *vil.Inst, v *vil.Value) ([]ir.Value, bool, error) {
	return c.gatherWords(in, v)
}

// gatherArg is gather for a call's argument, where a tuple travels as
// its elements rather than as its memory image cut into words.
//
// The two differ, and only here. swiftc's `sumTuple(_ t: (Int32,
// Int32))` reads w0 and w1, one register per element -- but `pairOf`,
// which returns the same tuple, packs it into x0 the way it packs a
// struct. A tuple parameter is the parameter list; a tuple result is
// a value.
func (c *fn) gatherArg(in *vil.Inst, v *vil.Value) ([]ir.Value, bool, error) {
	if rs, ok := tupleParts(v.Type()); ok {
		parts, held := c.multi[v]
		if !held {
			got, err := c.operand(in, v)
			if err != nil {
				return nil, false, err
			}
			parts = []ir.Value{got}
		}
		if len(parts) != len(rs) {
			return nil, false, c.fail(ErrUnsupported, in.Op(),
				"a tuple whose elements are not one register each")
		}
		return parts, true, nil
	}
	return c.gatherWords(in, v)
}

// gatherWords is the words a value is passed in: its memory image cut
// into eight-byte pieces, which is how a struct travels.
func (c *fn) gatherWords(in *vil.Inst, v *vil.Value) ([]ir.Value, bool, error) {
	ls, ok := leavesOf(v.Type())
	if !ok || len(ls) < 2 {
		return nil, false, nil
	}
	st, _ := structOf(v.Type())
	words, ok := structWords(st)
	if !ok {
		return nil, false, c.fail(ErrUnsupported, in.Op(), whyNoRegister(v.Type()))
	}
	// A value that lives in memory is already in the form a call
	// wants: its bytes, cut into words. So the words are loaded
	// rather than packed out of registers it is not in -- an array's
	// element comes back through storage, and a String read out of
	// one is this.
	if from, ok := c.mem[v]; ok {
		regs := make([]ir.Value, 0, len(words))
		for i := range words {
			regs = append(regs, c.b.I64.Load(c.fieldAddr(from, int64(i)*8)))
		}
		return regs, true, nil
	}
	fields, ok := c.multi[v]
	if !ok {
		// One register standing for the whole struct, which happens
		// where every field but one holds nothing.
		got, err := c.operand(in, v)
		if err != nil {
			return nil, false, err
		}
		fields = []ir.Value{got}
	}
	regs, ok := c.packStruct(fields, words)
	if !ok {
		return nil, false, c.fail(ErrUnsupported, in.Op(), whyNoRegister(v.Type()))
	}
	return regs, true, nil
}

func (c *fn) openBlock(b *vil.Block) error {
	if b.IsEntry() {
		out := c.out.Entry()
		c.blocks[b] = out
		// Taking a struct apart emits instructions, and they belong
		// at the top of the entry block — before anything reads a
		// field, which is everything. The builder has no block until
		// it is given one, and define() gives it each block in turn
		// later; this is the one place that has to say so early.
		c.b = out
		// The entry block's parameters are the function's. A struct
		// arrived as several of them and has to be taken apart before
		// anything reads a field: what the body knows about is the
		// fields, and what the caller sent is the words.
		params := c.out.Params()
		i := 0
		// The first parameter is the caller's storage when the result
		// comes back by address. It is not one of the VIL function's
		// own arguments -- the source returns a value and says
		// nothing about where it goes -- so it is taken before them
		// and remembered for the return.
		if c.returnsIndirectly() && len(params) > 0 {
			if p, ok := ir.Wrap(params[0]).(ir.Ptr); ok {
				c.sret, c.hasSRet = p, true
				i = 1
			}
		}
		for _, a := range b.Args() {
			if empty(a.Type()) {
				continue
			}
			// A parameter too wide for registers arrived as its
			// address, and stays there: the body reads its fields
			// out of memory.
			if _, yes := indirect(a.Type()); yes {
				if i >= len(params) {
					return c.fail(ErrUnsupported, vil.Op(""), "more entry arguments than parameters")
				}
				p, ok := ir.Wrap(params[i]).(ir.Ptr)
				if !ok {
					return c.fail(ErrType, vil.Op(""), "a wide argument that is not a pointer")
				}
				c.mem[a] = p
				i++
				continue
			}
			// A tuple arrived as its elements, one parameter each.
			if rs, ok := tupleParts(a.Type()); ok {
				if i+len(rs) > len(params) {
					return c.fail(ErrUnsupported, vil.Op(""),
						"more entry arguments than parameters")
				}
				parts := make([]ir.Value, 0, len(rs))
				for range rs {
					parts = append(parts, ir.Wrap(params[i]))
					i++
				}
				if len(parts) == 1 {
					c.def(a, parts[0])
				} else {
					c.multi[a] = parts
				}
				continue
			}
			n, wide := directWords(a.Type())
			if !wide {
				if i >= len(params) {
					return c.fail(ErrUnsupported, vil.Op(""), "more entry arguments than parameters")
				}
				c.values[a] = ir.Wrap(params[i])
				i++
				continue
			}
			if i+n > len(params) {
				return c.fail(ErrUnsupported, vil.Op(""), "more entry arguments than parameters")
			}
			regs := make([]ir.Value, 0, n)
			for _, p := range params[i : i+n] {
				regs = append(regs, ir.Wrap(p))
			}
			i += n
			if err := c.spreadInto(a, regs); err != nil {
				return err
			}
		}
		return nil
	}
	out := c.out.Block(b.Label())
	c.blocks[b] = out
	for i, a := range b.Args() {
		if empty(a.Type()) {
			continue
		}
		// One argument may be several registers: a case's payload
		// that is a tuple arrives as its scalars, the way a struct
		// parameter arrives as its words. What the block declares has
		// to match what every edge into it passes.
		if ls, ok := leavesOf(a.Type()); ok && len(ls) > 1 {
			parts := make([]ir.Value, 0, len(ls))
			for j, l := range ls {
				r, ok := machineOf(l.typ)
				if !ok {
					return c.fail(ErrType, vil.Op(""), l.typ.String())
				}
				parts = append(parts, out.Param(r.reg,
					b.Label()+"."+itoa(i)+"."+itoa(j)))
			}
			c.multi[a] = parts
			continue
		}
		r, ok := machine(a.Type())
		if !ok {
			return c.fail(ErrType, vil.Op(""), a.Type().String())
		}
		c.values[a] = out.Param(r.reg, b.Label()+"."+itoa(i))
	}
	return nil
}

// value is the register a VIL value is held in. A value of a type that
// holds nothing has no register, and ok is false: callers drop it
// rather than passing a poison value along.
func (c *fn) value(v *vil.Value) (ir.Value, bool) {
	if v == nil || empty(v.Type()) {
		return nil, false
	}
	got, ok := c.values[v]
	return got, ok
}

// def records what a VIL value became.
func (c *fn) def(v *vil.Value, got ir.Value) {
	if v == nil {
		return
	}
	c.values[v] = got
}

// forward makes a VIL value an alias for another's register. This is
// how the instructions that only change what a value is called --
// struct around a single field, struct_extract back out of it -- cost
// nothing at all.
func (c *fn) forward(dst, src *vil.Value) {
	if got, ok := c.value(src); ok {
		c.def(dst, got)
		return
	}
	// A value is not always one register. A struct of one field is
	// that field, and if the field is itself a struct of several then
	// what has to be carried across is all of them -- forwarding only
	// the single-register case left `struct Outer { var inner: Inner }`
	// holding nothing, and reading a field of it failed a long way
	// from here.
	if parts, ok := c.multi[src]; ok {
		c.multi[dst] = parts
		return
	}
	if p, ok := c.mem[src]; ok {
		c.mem[dst] = p
	}
}

// args are the registers a branch passes, in order, minus the ones
// that hold nothing.
func (c *fn) args(vs []*vil.Value) []ir.Value {
	out := make([]ir.Value, 0, len(vs))
	for _, v := range vs {
		// One value may be several registers -- an optional is its
		// payload and a tag -- and the block it goes to declares one
		// parameter for each. What the branch supplies has to match.
		if parts, ok := c.multi[v]; ok {
			out = append(out, parts...)
			continue
		}
		if got, ok := c.value(v); ok {
			out = append(out, got)
		}
	}
	return out
}

// trapBlock is where an arithmetic overflow or a failed precondition
// ends up. Swift's cond_fail is a trap: there is nothing to unwind to
// and no error to throw, which is the whole point of it.
func (c *fn) trapBlock() *ir.Block {
	if c.trap == nil {
		c.trap = c.out.Block("trap")
		c.trap.Trap()
	}
	return c.trap
}

func needsTrap(f *vil.Func) bool {
	for _, b := range f.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() == vil.CondFail {
				return true
			}
		}
	}
	return false
}

func (c *fn) fail(err error, op vil.Op, what string) error {
	return &Error{Err: err, Func: c.src.SourceName(), Op: op, What: what}
}

// returnsIndirectly reports whether this function's result comes back
// through storage the caller set aside.
func (c *fn) returnsIndirectly() bool {
	t := c.src.Type()
	if t == nil {
		return false
	}
	for _, res := range t.Results {
		if res.Convention == vil.ResultOut {
			return true
		}
		if _, yes := indirect(res.Type); yes {
			return true
		}
	}
	return false
}
