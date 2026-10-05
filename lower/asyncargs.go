package lower

import (
	"fmt"
	"strings"

	"github.com/vertex-language/ir"

	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Async arguments past the registers.
//
// Every call to an async function is a tail call (see async.go), and a
// tail call has nowhere to put an argument that does not fit in a
// register: the caller's frame is gone by the time the callee runs, and
// the stack past it is the caller's caller's. swiftc answers with
// swifttailcc, whose callee pops what it was passed. This compiler answers
// with the one piece of memory the caller has made for the callee and
// both can find: the callee's context. What does not fit in the
// argument registers is written at the end of it, a word each, in order:
//
//	child := task_alloc(size)
//	child[size - overflowBytes(n) + 8*k] = the k-th argument past the registers
//	tail_call f(child, the arguments that fit)
//
// The end rather than a fixed offset, because the callee's layout is its
// own business and the size is the one thing about it every caller knows,
// even through a function value or across modules (see asyncRecord). A
// callee with such arguments makes its frame that much bigger, and reads
// them back at its entry, where they join the arguments that came in
// registers as though they all had.
//
// What fits is decided by the same rule on both sides -- the target's
// argument registers, counted over the signature -- so an argument that
// fits travels exactly where swiftc's would, and only a call swiftc would
// have spilled to the stack looks different.

// An argBudget is how many arguments a target passes in registers.
type argBudget struct {
	ints, floats int
	// shared is a target whose integer and floating-point arguments
	// take positions from one sequence, as Windows x64's four do.
	shared bool
	// specials is whether the context, self and indirect-result
	// registers come out of the argument sequence. On arm64 they are
	// X22, X20 and X8, beside it; on x86-64 this compiler's backend
	// passes them as ordinary arguments.
	specials bool
}

func (l *lowerer) argBudget() argBudget {
	use := l.target.Use()
	switch {
	case strings.HasPrefix(use, "aarch64"):
		return argBudget{ints: 8, floats: 8}
	case use == "x86_64/windows":
		return argBudget{ints: 4, floats: 4, shared: true, specials: true}
	case strings.HasPrefix(use, "x86_64"):
		return argBudget{ints: 6, floats: 8, specials: true}
	}
	// No registers promised: everything past the context goes through it.
	return argBudget{specials: true}
}

// specialParam reports whether a parameter travels in a register of its
// own by declaration: the context, self, or an indirect result.
func specialParam(attrs []ir.ParamAttr) bool {
	for _, a := range attrs {
		if a.IsSwiftAsync() || a.IsSwiftSelf() || a.IsSwiftIndirectResult() || a.IsSRet() {
			return true
		}
	}
	return false
}

func floatReg(t ir.RegType) bool {
	switch t {
	case ir.TypeF64, ir.TypeF32, ir.TypeF16, ir.TypeBF16:
		return true
	}
	return false
}

// asyncKeep says, for each parameter of an async signature, whether it
// travels in a register. The rest go through the callee's context.
func (l *lowerer) asyncKeep(ps []ir.Param) []bool {
	b := l.argBudget()
	keep := make([]bool, len(ps))
	ints, floats := 0, 0
	for i, p := range ps {
		special := specialParam(p.Attrs)
		if special && !b.specials {
			keep[i] = true
			continue
		}
		switch {
		case b.shared:
			keep[i] = ints < b.ints
			ints++
		case floatReg(p.Type):
			keep[i] = floats < b.floats
			floats++
		default:
			keep[i] = ints < b.ints
			ints++
		}
		// The context, self and the result's storage are never moved:
		// they are how the callee finds everything else.
		if special {
			keep[i] = true
		}
	}
	return keep
}

// reduceAsync is an async signature with the parameters that do not fit
// in registers taken out, and how many were.
func (l *lowerer) reduceAsync(full *ir.Sig) (*ir.Sig, int) {
	keep := l.asyncKeep(full.Params())
	n := 0
	for _, k := range keep {
		if !k {
			n++
		}
	}
	if n == 0 {
		return full, 0
	}
	out := ir.NewSig().Conv(full.CallConv())
	for i, p := range full.Params() {
		if keep[i] {
			out.Param(p.Type, p.Attrs...)
		}
	}
	for _, r := range full.Rets() {
		out.Ret(r.Type, r.Attrs...)
	}
	return out, n
}

// overflowBytes is the room n arguments past the registers take at the
// end of a context: a word each, rounded to keep the frame 16-aligned.
func overflowBytes(n int) int64 { return roundUp(int64(8*n), 16) }

// asyncParams is an async function's parameters as its full signature
// lists them: those that came in registers, and those read back out of
// the end of its context, which is size bytes. ctxAt is the index of the
// context among the registers.
func asyncParams(b *ir.Block, full *ir.Sig, keep []bool, regs []ir.Value, ctxAt int, size int64) ([]ir.Value, error) {
	if ctxAt < 0 || ctxAt >= len(regs) {
		return nil, fmt.Errorf("an async function with no context")
	}
	ctx, ok := regs[ctxAt].(ir.Ptr)
	if !ok {
		return nil, fmt.Errorf("a context that is not a pointer")
	}
	n := 0
	for _, k := range keep {
		if !k {
			n++
		}
	}
	base := size - overflowBytes(n)
	out := make([]ir.Value, 0, len(keep))
	r, k := 0, 0
	for i, p := range full.Params() {
		if keep[i] {
			if r >= len(regs) {
				return nil, fmt.Errorf("fewer registers than the signature keeps")
			}
			out = append(out, regs[r])
			r++
			continue
		}
		v, err := loadWord(b, p.Type, b.Ptr.Add(ctx, b.I64.Const(base+int64(8*k))))
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		k++
	}
	return out, nil
}

// storeOverflow writes the arguments a call does not pass in registers
// into the end of the callee's context, which is size bytes, and answers
// with the ones it does pass.
func storeOverflow(b *ir.Block, keep []bool, args []ir.Value, child ir.Ptr, size ir.I64) ([]ir.Value, error) {
	if len(keep) != len(args) {
		return nil, fmt.Errorf("an async call of %d arguments to a signature of %d", len(args), len(keep))
	}
	n := 0
	for _, k := range keep {
		if !k {
			n++
		}
	}
	if n == 0 {
		return args, nil
	}
	base := b.Ptr.Add(child, b.I64.Sub(size, b.I64.Const(overflowBytes(n))))
	var regs []ir.Value
	k := 0
	for i, a := range args {
		if keep[i] {
			regs = append(regs, a)
			continue
		}
		if err := storeWord(b, a, b.Ptr.Add(base, b.I64.Const(int64(8*k)))); err != nil {
			return nil, err
		}
		k++
	}
	return regs, nil
}

// loadWord reads one register's worth out of the frame, as storeWord
// wrote it.
func loadWord(b *ir.Block, t ir.RegType, at ir.Ptr) (ir.Value, error) {
	switch t {
	case ir.TypeI1:
		return b.I32.Ne(b.I32.Load(at), b.I32.Const(0)), nil
	case ir.TypeI64:
		return b.I64.Load(at), nil
	case ir.TypeI32:
		return b.I32.Load(at), nil
	case ir.TypeF64:
		return b.F64.Load(at), nil
	case ir.TypeF32:
		return b.F32.Load(at), nil
	case ir.TypeF16:
		return b.F16().Load(at), nil
	case ir.TypeBF16:
		return b.BF16().Load(at), nil
	case ir.TypePtr:
		return b.Ptr.Load(at), nil
	}
	return nil, fmt.Errorf("an async argument passed in a %s register", t)
}

// asyncOverflow is how many of f's arguments travel in its context.
func (l *lowerer) asyncOverflow(f *sil.Func) (int, error) {
	if f.Type() == nil || !f.Type().Async {
		return 0, nil
	}
	full, err := l.fullSignature(f.Name(), f.Type())
	if err != nil {
		return 0, err
	}
	_, n := l.reduceAsync(full)
	return n, nil
}

// entryParams is a function's parameters as its full signature lists
// them, read at the top of b: for an async function whose arguments do
// not all fit in registers, the rest out of its context.
func (l *lowerer) entryParams(f *sil.Func, out *ir.Func, b *ir.Block) ([]ir.Value, error) {
	var regs []ir.Value
	for _, p := range out.Params() {
		regs = append(regs, ir.Wrap(p))
	}
	if f.Type() == nil || !f.Type().Async {
		return regs, nil
	}
	full, err := l.fullSignature(f.Name(), f.Type())
	if err != nil {
		return nil, err
	}
	keep := l.asyncKeep(full.Params())
	ctxAt := -1
	r := 0
	for i, p := range full.Params() {
		if !keep[i] {
			continue
		}
		for _, a := range p.Attrs {
			if a.IsSwiftAsync() {
				ctxAt = r
			}
		}
		r++
	}
	if r == len(full.Params()) {
		return regs, nil
	}
	return asyncParams(b, full, keep, regs, ctxAt, l.asyncSizes[f.Name()])
}

// overflowArgs is an async call's arguments as the callee's registers
// take them, the rest written into the end of its context: child, of
// size bytes.
func (c *fn) overflowArgs(in *sil.Inst, callee *sil.Value, call []ir.Value, child ir.Ptr, size ir.I64) ([]ir.Value, error) {
	full, err := c.l.calleeFullSig(callee)
	if err != nil {
		return nil, c.fail(ErrType, in.Op(), err.Error())
	}
	keep := c.l.asyncKeep(full.Params())
	all := true
	for _, k := range keep {
		all = all && k
	}
	if all {
		return call, nil
	}
	regs, err := storeOverflow(c.b, keep, call, child, size)
	if err != nil {
		return nil, c.fail(ErrUnsupported, in.Op(), err.Error())
	}
	return regs, nil
}

// calleeFullSig is the full signature of what a call calls: a function
// or a method out of a table, by its SIL type, or a function value, by
// its Swift one.
func (l *lowerer) calleeFullSig(v *sil.Value) (*ir.Sig, error) {
	t := v.Type()
	if !t.IsValid() || t.Formal() == nil {
		return nil, fmt.Errorf("the callee has no function type")
	}
	switch f := t.Formal().Underlying().(type) {
	case *types.Signature:
		return l.fullFuncSig(f)
	case *sil.FuncType:
		return l.fullSignature("", f)
	}
	return nil, fmt.Errorf("the callee is not a function")
}
