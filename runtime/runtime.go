// Package runtime is what a compiled program calls: allocation, and
// the two reference-count operations that stand for ownership once
// ownership has been lowered away.
//
// It is a VIR module this compiler builds for itself, not a library
// somebody has to compile first. That is worth saying because the
// alternative was assumed for a while and written into a comment in
// lower/: that the runtime would be C, compiled by vcc and linked
// alongside. It does not need to be. Everything ARC does at this
// level is a load, an atomic add, a compare and a branch, and VIR has
// all four — so the runtime is emitted by the same backend as the
// program it serves, for whatever target that program is built for,
// with no second toolchain in the picture.
//
// What it does need from the platform is memory. malloc and free are
// declared here and resolved by the linker against libSystem, which
// is a link-time dependency on the platform's libc rather than a
// compile-time dependency on a C compiler.
//
// # The object
//
// An instance is a header and then its stored properties:
//
//	+0   metadata   what the object is — unused so far, and zero
//	+8   refcount   how many references are held to it
//	+16  the first stored property
//
// Two words, which is what lower/ already assumes when it computes a
// property's address, and the one thing the compiler and the runtime
// have to agree about.
package runtime

import (
	"github.com/vertex-language/ir"
)

// The symbols a compiled program refers to. lower/ emits calls to
// these names, and they are spelled here so that the two cannot
// disagree about them.
const (
	Alloc   = "vertex_alloc"
	Retain  = "vertex_retain"
	Release = "vertex_release"
)

// The object header, in words and in bytes. lower/ has the same
// number written down for the same reason.
const (
	HeaderWords = 2
	HeaderBytes = HeaderWords * 8
)

// Options are what the caller decides.
type Options struct {
	// SymbolPrefix is what the target writes in front of a symbol.
	// The runtime's definitions and the program's calls to them have
	// to agree, so this is the same value lower/ was given.
	SymbolPrefix string
}

// Module builds the runtime for a target.
//
// The module is complete and verifiable on its own: nothing in it
// refers to the program it will be linked with, and the program
// refers to it only by the three names above.
func Module(target ir.Target, opts Options) (*ir.Module, error) {
	m := ir.NewModule("vertex_runtime", target)
	b := &builder{m: m, prefix: opts.SymbolPrefix}

	// The platform supplies memory and nothing else.
	b.malloc = m.ImportFunc(b.sym("malloc"), ir.NewSig().Param(ir.TypeI64).Ret(ir.TypePtr)).NoUnwind()
	b.free = m.ImportFunc(b.sym("free"), ir.NewSig().Param(ir.TypePtr)).NoUnwind()

	b.alloc()
	b.retain()
	b.release()

	if err := m.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

type builder struct {
	m      *ir.Module
	prefix string
	malloc ir.Callee
	free   ir.Callee
}

func (b *builder) sym(name string) string { return b.prefix + name }

// alloc makes an instance: room for the header and the properties,
// with the header set to one live reference.
//
//	vertex_alloc(size) -> ptr
//
// The count starts at one because the caller is holding it. A newly
// made object nobody keeps is released by the code that made it,
// which is what vil/gen's cleanup stack already arranges.
func (b *builder) alloc() {
	f := b.m.Func(b.sym(Alloc)).Export().NoUnwind()
	size := f.ParamI64("size")
	f.ReturnsPtr()

	e := f.Entry()
	total := e.I64.Add(size, e.I64.Const(HeaderBytes))
	obj := e.Call(b.malloc, total).Value(0).(ir.Ptr)

	// The metadata word is zero until there is something to put in
	// it. It is written rather than left as whatever malloc returned,
	// so that an instance is the same shape however it was allocated.
	e.I64.Store(e.I64.Const(0), obj)
	e.I64.Store(e.I64.Const(1), b.refCountAddr(e, obj))
	e.Return(obj)
}

// retain adds a reference.
//
// Relaxed is the right ordering for an increment: the reference being
// counted was already reachable by whoever is counting it, so there
// is nothing this operation makes visible that was not visible
// before. It is the ordering Swift's own retain uses, and libstdc++'s
// and libc++'s shared_ptr before it.
func (b *builder) retain() {
	f := b.m.Func(b.sym(Retain)).Export().NoUnwind()
	obj := f.ParamPtr("obj")

	e := f.Entry()
	e.I64.AtomicRmwAdd(e.I64.Const(1), b.refCountAddr(e, obj), ir.Monotonic)
	e.Return()
}

// release drops a reference, and frees the object when it was the
// last one.
//
// The decrement is a release so that everything this thread did to
// the object happens before another thread's decrement sees the lower
// count. The thread that takes the count to zero then acquires,
// which pairs with every other thread's release and makes their
// writes visible before the memory is handed back. Getting this pair
// wrong is a use-after-free that appears only under contention, which
// is the reason to write down why rather than only what.
func (b *builder) release() {
	f := b.m.Func(b.sym(Release)).Export().NoUnwind()
	obj := f.ParamPtr("obj")

	e := f.Entry()
	last := f.Block("last")
	alive := f.Block("alive")

	// AtomicRmwSub answers with the count as it was, so one means
	// this call took it to zero.
	was := e.I64.AtomicRmwSub(e.I64.Const(1), b.refCountAddr(e, obj), ir.Release)
	e.BrIf(e.I64.Eq(was, e.I64.Const(1)), last.To(), alive.To())

	last.Fence(ir.Acquire)
	last.Call(b.free, obj)
	last.Return()

	alive.Return()
}

// refCountAddr is where the count lives: one word past the start of
// the object.
func (b *builder) refCountAddr(e *ir.Block, obj ir.Ptr) ir.Ptr {
	return e.Ptr.Add(obj, e.I64.Const(8))
}
