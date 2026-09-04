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
	for _, b := range f.Blocks() {
		c.b = c.blocks[b]
		for _, in := range b.Insts() {
			if err := c.inst(in); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *fn) openBlock(b *vil.Block) error {
	if b.IsEntry() {
		out := c.out.Entry()
		c.blocks[b] = out
		// The entry block's parameters are the function's.
		params := c.out.Params()
		i := 0
		for _, a := range b.Args() {
			if empty(a.Type()) {
				continue
			}
			if i >= len(params) {
				return c.fail(ErrUnsupported, vil.Op(""), "more entry arguments than parameters")
			}
			c.values[a] = ir.Wrap(params[i])
			i++
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
	return &Error{Err: err, Func: c.src.Name(), Op: op, What: what}
}
