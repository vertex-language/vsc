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
			if in.Op() != vil.AllocStack {
				continue
			}
			if err := c.allocStack(in); err != nil {
				return err
			}
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
	ls, ok := leavesOf(v.Type())
	if !ok || len(ls) < 2 {
		return nil, false, nil
	}
	st, _ := structOf(v.Type())
	words, ok := structWords(st)
	if !ok {
		return nil, false, c.fail(ErrUnsupported, in.Op(), whyNoRegister(v.Type()))
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
		for _, a := range b.Args() {
			if empty(a.Type()) {
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
	}
}

// args are the registers a branch passes, in order, minus the ones
// that hold nothing.
func (c *fn) args(vs []*vil.Value) []ir.Value {
	out := make([]ir.Value, 0, len(vs))
	for _, v := range vs {
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
