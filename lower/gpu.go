package lower

import (
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Kernels, as lowering sees them (proposed_vertex_kernel.md §8).
//
// On the host, a kernel is the function it is, and beside it the compiler
// writes two things the gpu runtime unit reads:
//
//   - its CPU thunk, <kernel>$cpu(slots): the kernel called with each of
//     its parameters loaded from the slot the runtime filled, which is how
//     the CPU device runs a work-item;
//   - its descriptor, <kernel>$gpu: the name of its entry in the Metal
//     library, the library, the thunk, and how many slots a launch fills
//     (stdlib/runtime/gpu/gpu.cpp says the layout).
//
// A launch's kernel is named by a builtin (gen.KernelDescriptorBuiltin)
// that is the descriptor's address: this module's own, or another's.
//
// For a device, the same SIL is lowered with Device set: each call to a
// gpu intrinsic -- vertex_gpu_workitem_id_x and the rest, the functions
// the built-in gpu module declares and does not define -- is the VIR verb
// it names, and each kernel gets an entry, callconv kernel, that calls it.

// KernelDescriptorBuiltin is gen.KernelDescriptorBuiltin; lower does not
// import gen.
const kernelDescriptorBuiltin = "vertex_gpu_kernel_descriptor:"

// KernelEntry is the name of the entry point each kernel's device
// library holds: one library per kernel, so one name serves.
const KernelEntry = "vertex_kernel"

// A KernelImage is what the device compile built for one kernel.
type KernelImage struct {
	// Metallib is the kernel as a Metal library, holding KernelEntry;
	// nil where it could not be built for Apple's GPUs, which run it on
	// the CPU instead.
	Metallib []byte
	// Barrier is whether the kernel waits at a barrier, which the CPU
	// device runs with a thread per work-item.
	Barrier bool
}

const kernelUsesBarrier = 1

// kernelDescriptor is the descriptor of the kernel whose SIL name is
// kernel: defined here when the kernel is this module's, imported when
// it is another's.
func (l *lowerer) kernelDescriptor(kernel string) ir.Symbol {
	name := l.sym(kernel + "$gpu")
	if g, ok := l.kernelDescs[name]; ok {
		return g
	}
	if l.kernelDescs == nil {
		l.kernelDescs = map[string]ir.Symbol{}
	}
	if img, ok := l.kernels[kernel]; ok {
		if g := l.defineKernelDescriptor(kernel, name, img); g != nil {
			l.kernelDescs[name] = g
			return g
		}
	}
	ext := l.out.ImportGlobal(name, ir.StorePtr.FType())
	l.kernelDescs[name] = ext
	return ext
}

// defineKernelDescriptor writes a kernel's descriptor and its CPU thunk.
func (l *lowerer) defineKernelDescriptor(kernel, name string, img *KernelImage) *ir.Global {
	fn := l.defs[kernel]
	if base, mask, isMap := splitMapVariant(kernel); isMap {
		fn = l.defs[base]
		if fn == nil {
			return nil
		}
		// The slots a map launch fills: each argument's, then the output's.
		params := l.mapParams(base, mask)
		if params == nil {
			return nil
		}
		slots := 2 // the output's pointer and count
		for _, p := range params {
			if p.mapped {
				slots += 2
			} else {
				slots++
			}
		}
		return l.writeKernelDescriptor(kernel, name, img, slots, fn)
	}
	if fn == nil {
		return nil
	}
	return l.writeKernelDescriptor(kernel, name, img, len(fn.Params()), fn)
}

// writeKernelDescriptor writes a descriptor of params slots, and the thunk.
func (l *lowerer) writeKernelDescriptor(kernel, name string, img *KernelImage, params int, fn *ir.Func) *ir.Global {
	var thunk *ir.Func
	if base, mask, isMap := splitMapVariant(kernel); isMap {
		thunk = l.mapThunk(kernel, base, mask)
		if thunk == nil {
			return nil
		}
	} else {
		thunk = l.kernelThunk(kernel, fn)
	}
	entry := l.out.Global(l.sym(kernel+"$gpu_name"), ir.RO, ir.Array(uint64(len(KernelEntry)+1), ir.StoreI8.FType())).
		Init(ir.Str(KernelEntry + "\x00"))
	entry.Internal()

	if l.kernelRecord == nil {
		rec := l.out.Struct("vertex_gpu_kernel")
		rec.Field("name", ir.StorePtr.FType())
		rec.Field("metallib", ir.StorePtr.FType())
		rec.Field("metallibSize", ir.StoreI64.FType())
		rec.Field("cpu", ir.StorePtr.FType())
		rec.Field("flags", ir.StoreI64.FType())
		rec.Field("params", ir.StoreI64.FType())
		l.kernelRecord = rec
	}
	var flags int64
	if img.Barrier {
		flags |= kernelUsesBarrier
	}
	lib := ir.Lit(ir.Int(0))
	if len(img.Metallib) > 0 {
		g := l.out.Global(l.sym(kernel+"$gpu_metallib"), ir.RO, ir.Array(uint64(len(img.Metallib)), ir.StoreI8.FType())).
			Init(ir.Str(string(img.Metallib))).Align(16)
		g.Internal()
		lib = ir.RelocInit(g)
	}
	g := l.out.Global(name, ir.RO, l.kernelRecord.FType()).
		Init(ir.Fields(
			ir.Val("name", ir.RelocInit(entry)),
			ir.Val("metallib", lib),
			ir.Val("metallibSize", ir.Lit(ir.Int(int64(len(img.Metallib))))),
			ir.Val("cpu", ir.RelocInit(thunk)),
			ir.Val("flags", ir.Lit(ir.Int(flags))),
			ir.Val("params", ir.Lit(ir.Int(int64(params)))),
		)).
		Align(8)
	g.Export()
	return g
}

// kernelThunk is <kernel>$cpu(slots): the kernel called with parameter i
// loaded from slots[i], which the runtime points at the argument.
func (l *lowerer) kernelThunk(kernel string, fn *ir.Func) *ir.Func {
	f := l.out.Func(l.sym(kernel + "$cpu"))
	slots := f.ParamPtr("slots")
	b := f.Entry()
	args := make([]ir.Value, 0, len(fn.Params()))
	for i, p := range fn.Params() {
		at := b.Ptr.Load(b.Ptr.Add(slots, b.I64.Const(int64(8*i))))
		switch p.Type() {
		case ir.TypeI1:
			args = append(args, b.I32.Ne(b.I32.ULoad8(at), b.I32.Const(0)))
		case ir.TypeI32:
			args = append(args, b.I32.Load(at))
		case ir.TypeI64:
			args = append(args, b.I64.Load(at))
		case ir.TypeF32:
			args = append(args, b.F32.Load(at))
		case ir.TypeF64:
			args = append(args, b.F64.Load(at))
		case ir.TypePtr:
			args = append(args, b.Ptr.Load(at))
		}
	}
	b.Call(fn, args...)
	b.Return()
	return f
}

// kernelEntries writes, for a device, each kernel's entry: callconv
// kernel, and a call to the kernel.
//
// The entry's parameters are the kernel's as a device binds them, which
// is not how a function passes them: a span is a struct of a pointer and
// a count, which a call passes as two words, and a device binds a word
// as a constant. So the entry takes a span's pointer as a pointer -- a
// buffer -- and makes the word the kernel wants from it.
func (l *lowerer) kernelEntries() error {
	for _, kernel := range l.deviceKernels {
		if base, mask, isMap := splitMapVariant(kernel); isMap {
			if err := l.mapEntry(base, mask); err != nil {
				return err
			}
			continue
		}
		fn := l.defs[kernel]
		sf := l.module.Lookup(kernel)
		if fn == nil || sf == nil {
			return &Error{Err: ErrUnsupported, What: "no kernel named " + kernel}
		}
		words := fn.Params()
		// Each of the kernel's words, and whether the entry takes it as
		// a pointer.
		asPointer := make([]bool, len(words))
		at := 0
		for _, sp := range sf.Type().Params {
			fields := structFieldTypes(sp.Type.Formal())
			if len(fields) > 1 && at+len(fields) <= len(words) {
				for j, ft := range fields {
					if _, isPtr := ft.Underlying().(*types.Pointer); isPtr && words[at+j].Type() == ir.TypeI64 {
						asPointer[at+j] = true
					}
				}
				at += len(fields)
				continue
			}
			at++
		}
		f := l.out.Func(KernelEntry).CallConv(ir.Kernel)
		f.Export()
		params := make([]ir.Value, len(words))
		for i, w := range words {
			if asPointer[i] {
				params[i] = f.ParamPtr("p" + itoa(i))
			} else {
				params[i] = f.ParamOf(w.Type(), "p"+itoa(i))
			}
		}
		b := f.Entry()
		args := make([]ir.Value, len(words))
		for i, p := range params {
			if asPointer[i] {
				args[i] = b.I64.FromPtr(p.(ir.Ptr))
			} else {
				args[i] = p
			}
		}
		b.Call(fn, args...)
		b.Return()
	}
	return nil
}

// structFieldTypes is the stored fields' types of a struct, or of an
// instance of a generic one; nil for anything else.
func structFieldTypes(t types.Type) []types.Type {
	if gi, ok := t.(*types.GenericInstance); ok {
		t = gi.Base
	}
	if t == nil {
		return nil
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	out := make([]types.Type, 0, len(st.Fields))
	for _, f := range st.Fields {
		if f != nil {
			out = append(out, f.Type)
		}
	}
	return out
}

// gpuIntrinsic lowers a call to one of the built-in gpu module's
// intrinsics as the VIR it names, on a device. done is false for any
// other call, and for one lowering leaves as a call (vertex_gpu_shared,
// which the device compile resolves once the kernel is inlined).
func (c *fn) gpuIntrinsic(in *sil.Inst) (bool, error) {
	if !c.l.device {
		return false, nil
	}
	name := c.refNames[in.Args()[0]]
	if !strings.HasPrefix(name, "vertex_gpu_") || name == "vertex_gpu_shared" {
		return false, nil
	}
	var args []ir.Value
	for _, a := range in.Args()[1:] {
		v, err := c.operand(in, a)
		if err != nil {
			return true, err
		}
		args = append(args, v)
	}
	i32 := func(i int) ir.I32 { v, _ := args[i].(ir.I32); return v }
	i1 := func(i int) ir.I1 { v, _ := args[i].(ir.I1); return v }
	ptr := func(i int) ir.Ptr { v, _ := args[i].(ir.Ptr); return v }
	f32 := func(i int) ir.F32 { v, _ := args[i].(ir.F32); return v }
	all := c.b.I32.Const(-1)
	b := c.b
	axis := func(suffix string) (ir.Axis, bool) {
		switch {
		case strings.HasSuffix(name, "_x"):
			return ir.X, true
		case strings.HasSuffix(name, "_y"):
			return ir.X + 1, true
		case strings.HasSuffix(name, "_z"):
			return ir.X + 2, true
		}
		return 0, false
	}
	var v ir.Value
	switch {
	case strings.HasPrefix(name, "vertex_gpu_workitem_id_"):
		a, _ := axis(name)
		v = b.I32.WorkitemID(a)
	case strings.HasPrefix(name, "vertex_gpu_workgroup_id_"):
		a, _ := axis(name)
		v = b.I32.WorkgroupID(a)
	case strings.HasPrefix(name, "vertex_gpu_workgroup_size_"):
		a, _ := axis(name)
		v = b.I32.WorkgroupSize(a)
	case strings.HasPrefix(name, "vertex_gpu_num_workgroups_"):
		a, _ := axis(name)
		v = b.I32.NumWorkgroups(a)
	case name == "vertex_gpu_lane_id":
		v = b.I32.LaneID()
	case name == "vertex_gpu_wave_size":
		v = b.I32.WaveSize()
	case name == "vertex_gpu_barrier":
		b.Barrier()
		return true, nil
	case name == "vertex_gpu_trap":
		// Never returns: the unreachable after it is the trap.
		return true, nil
	case name == "vertex_gpu_shfl_idx":
		v = b.I32.WaveShflIdx(i32(0), i32(1), all)
	case name == "vertex_gpu_shfl_up":
		v = b.I32.WaveShflUp(i32(0), i32(1), all)
	case name == "vertex_gpu_shfl_down":
		v = b.I32.WaveShflDown(i32(0), i32(1), all)
	case name == "vertex_gpu_shfl_xor":
		v = b.I32.WaveShflXor(i32(0), i32(1), all)
	case name == "vertex_gpu_readfirstlane":
		v = b.I32.WaveReadFirstLane(i32(0))
	case name == "vertex_gpu_ballot":
		v = b.I64.WaveBallot(i1(0), all)
	case name == "vertex_gpu_wave_any":
		v = b.I1.WaveAny(i1(0), all)
	case name == "vertex_gpu_wave_all":
		v = b.I1.WaveAll(i1(0), all)
	case name == "vertex_gpu_atomic_add_i32", name == "vertex_gpu_atomic_add_u32":
		v = b.I32.AtomicRmwAdd(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_sub_i32":
		v = b.I32.AtomicRmwSub(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_and_i32":
		v = b.I32.AtomicRmwAnd(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_or_i32":
		v = b.I32.AtomicRmwOr(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_xor_i32":
		v = b.I32.AtomicRmwXor(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_xchg_i32":
		v = b.I32.AtomicRmwXchg(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_min_i32":
		v = b.I32.AtomicRmwSMin(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_max_i32":
		v = b.I32.AtomicRmwSMax(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_min_u32":
		v = b.I32.AtomicRmwUMin(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_max_u32":
		v = b.I32.AtomicRmwUMax(i32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_cas_i32":
		v = b.I32.AtomicCas(i32(1), i32(2), ptr(0), ir.Monotonic, ir.Monotonic, ir.DeviceScope)
	case name == "vertex_gpu_atomic_add_f32":
		v = b.F32.AtomicRmwAdd(f32(1), ptr(0), ir.Monotonic, ir.DeviceScope)
	default:
		return true, c.fail(ErrUnsupported, in.Op(), "a gpu intrinsic this compiler does not know: "+name)
	}
	if r := in.Result(); r != nil && v != nil {
		c.def(r, v)
	}
	return true, nil
}

// kernelDescriptorCall lowers the builtin a launch names its kernel by.
func (c *fn) kernelDescriptorCall(in *sil.Inst) (bool, error) {
	name := in.Aux().Name
	if !strings.HasPrefix(name, kernelDescriptorBuiltin) {
		return false, nil
	}
	kernel := strings.TrimPrefix(name, kernelDescriptorBuiltin)
	if r := in.Result(); r != nil {
		c.def(r, c.b.Ptr.GetAddr(c.l.kernelDescriptor(kernel)))
	}
	return true, nil
}

// ---- Map: the grid kernel an element kernel stands for (§4.2) ----
//
// An element kernel f(a, b, …) -> R, mapped with some of its parameters
// taken from buffers (the mask), is launched as a grid kernel the
// compiler writes here: work-item i reads element i of each mapped
// buffer, calls f with those and the broadcast arguments, and writes
// element i of the output. Its parameters are each of f's -- a mapped
// one as a buffer's pointer and count, a broadcast one as itself -- and
// then the output's pointer and count. The grid is the output's count;
// a work-item past it does nothing.

// splitMapVariant reads a map variant's name, <f>$map<mask>.
func splitMapVariant(name string) (base string, mask uint64, ok bool) {
	i := strings.LastIndex(name, "$map")
	if i < 0 {
		return "", 0, false
	}
	var m uint64
	digits := name[i+len("$map"):]
	if digits == "" {
		return "", 0, false
	}
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			return "", 0, false
		}
		m = m*10 + uint64(ch-'0')
	}
	return name[:i], m, true
}

// mapParam is one of f's parameters as a map variant sees it.
type mapParam struct {
	mapped bool
	reg    ir.RegType // what f takes it as
	elem   repr       // a mapped one's element, as it is in memory
	stride int64
}

// mapParams is f's parameters for a map variant, and nil where f is not
// an element kernel of scalars.
func (l *lowerer) mapParams(base string, mask uint64) []mapParam {
	sf := l.module.Lookup(base)
	fn := l.defs[base]
	if sf == nil || fn == nil || len(sf.Type().Params) != len(fn.Params()) {
		return nil
	}
	out := make([]mapParam, 0, len(fn.Params()))
	for i, sp := range sf.Type().Params {
		r, ok := machine(sp.Type)
		if !ok {
			return nil
		}
		mp := mapParam{mapped: mask&(1<<uint(i)) != 0, reg: fn.Params()[i].Type(), elem: r}
		mp.stride = elemStride(r)
		if mp.stride == 0 {
			return nil
		}
		out = append(out, mp)
	}
	return out
}

// mapResult is f's result as the output buffer holds it.
func (l *lowerer) mapResult(base string) (repr, bool) {
	sf := l.module.Lookup(base)
	if sf == nil || len(sf.Type().Results) != 1 {
		return repr{}, false
	}
	return machine(sf.Type().Results[0].Type)
}

// elemStride is how many bytes an element of the representation takes
// in a buffer.
func elemStride(r repr) int64 {
	if r.narrow() {
		return int64(r.width / 8)
	}
	switch r.reg {
	case ir.TypeI1:
		return 1
	case ir.TypeI32, ir.TypeF32:
		return 4
	case ir.TypeI64, ir.TypeF64, ir.TypePtr:
		return 8
	}
	return 0
}

// loadElem reads an element of the representation at p.
func loadElem(b *ir.Block, p ir.Ptr, r repr) ir.Value {
	if r.narrow() {
		switch {
		case r.width == 8 && r.signed:
			return b.I32.SLoad8(p)
		case r.width == 8:
			return b.I32.ULoad8(p)
		case r.signed:
			return b.I32.SLoad16(p)
		default:
			return b.I32.ULoad16(p)
		}
	}
	switch r.reg {
	case ir.TypeI1:
		return b.I32.Ne(b.I32.ULoad8(p), b.I32.Const(0))
	case ir.TypeI32:
		return b.I32.Load(p)
	case ir.TypeI64:
		return b.I64.Load(p)
	case ir.TypeF32:
		return b.F32.Load(p)
	case ir.TypeF64:
		return b.F64.Load(p)
	}
	return b.Ptr.Load(p)
}

// storeElem writes an element of the representation at p.
func storeElem(b *ir.Block, v ir.Value, p ir.Ptr, r repr) {
	if r.narrow() {
		n, _ := v.(ir.I32)
		if r.width == 8 {
			b.I32.Store8(n, p)
		} else {
			b.I32.Store16(n, p)
		}
		return
	}
	switch v := v.(type) {
	case ir.I1:
		b.I32.Store8(b.I32.Select(v, b.I32.Const(1), b.I32.Const(0)), p)
	case ir.I32:
		b.I32.Store(v, p)
	case ir.I64:
		b.I64.Store(v, p)
	case ir.F32:
		b.F32.Store(v, p)
	case ir.F64:
		b.F64.Store(v, p)
	case ir.Ptr:
		b.Ptr.Store(v, p)
	}
}

// mapBody fills a map variant's body from entry, given its arguments as
// the variant receives them -- in order, a mapped one as a pointer and a
// count, a broadcast one as itself, then the output's pointer and count
// -- and the work-item's index in the grid.
func (l *lowerer) mapBody(f *ir.Func, entry *ir.Block, base string, params []mapParam, res repr, args []ir.Value, index ir.I64) {
	fn := l.defs[base]
	at := 0
	type input struct {
		p      mapParam
		ptr    ir.Ptr
		scalar ir.Value
	}
	inputs := make([]input, len(params))
	for i, p := range params {
		if p.mapped {
			inputs[i] = input{p: p, ptr: args[at].(ir.Ptr)}
			at += 2
		} else {
			inputs[i] = input{p: p, scalar: args[at]}
			at++
		}
	}
	out := args[at].(ir.Ptr)
	count := args[at+1].(ir.I64)
	body := f.Block("element")
	done := f.Block("done")
	entry.BrIf(entry.I64.SLt(index, count), body.To(), done.To())
	call := make([]ir.Value, len(inputs))
	for i, in := range inputs {
		if !in.p.mapped {
			call[i] = in.scalar
			continue
		}
		addr := body.Ptr.Add(in.ptr, body.I64.Mul(index, body.I64.Const(in.p.stride)))
		call[i] = loadElem(body, addr, in.p.elem)
	}
	rs := body.Call(fn, call...)
	if rs.Len() > 0 {
		addr := body.Ptr.Add(out, body.I64.Mul(index, body.I64.Const(elemStride(res))))
		storeElem(body, rs.Value(0), addr, res)
	}
	body.Br(done.To())
	done.Return()
}

// mapEntry writes, for a device, a map variant's entry: callconv kernel.
func (l *lowerer) mapEntry(base string, mask uint64) error {
	params := l.mapParams(base, mask)
	res, ok := l.mapResult(base)
	if params == nil || !ok || elemStride(res) == 0 {
		return &Error{Err: ErrUnsupported, What: "an element kernel of numbers and bools, to map: " + base}
	}
	f := l.out.Func(KernelEntry).CallConv(ir.Kernel)
	f.Export()
	var args []ir.Value
	n := 0
	param := func(t ir.RegType) ir.Value {
		v := f.ParamOf(t, "p"+itoa(n))
		n++
		return v
	}
	for _, p := range params {
		if p.mapped {
			args = append(args, f.ParamPtr("p"+itoa(n)))
			n++
			args = append(args, param(ir.TypeI64))
		} else {
			args = append(args, param(p.reg))
		}
	}
	args = append(args, f.ParamPtr("p"+itoa(n)))
	n++
	args = append(args, param(ir.TypeI64))
	b := f.Entry()
	index := b.I64.Add(
		b.I64.Mul(b.I64.SExtI32(b.I32.WorkgroupID(ir.X)), b.I64.SExtI32(b.I32.WorkgroupSize(ir.X))),
		b.I64.SExtI32(b.I32.WorkitemID(ir.X)))
	l.mapBody(f, b, base, params, res, args, index)
	return nil
}

// mapThunk is a map variant on the CPU device: its arguments from the
// slots, as a kernel's thunk has them, and its index from the runtime.
func (l *lowerer) mapThunk(kernel, base string, mask uint64) *ir.Func {
	params := l.mapParams(base, mask)
	res, ok := l.mapResult(base)
	if params == nil || !ok || elemStride(res) == 0 {
		return nil
	}
	f := l.out.Func(l.sym(kernel + "$cpu"))
	slots := f.ParamPtr("slots")
	b := f.Entry()
	slot := 0
	load := func(t ir.RegType) ir.Value {
		at := b.Ptr.Load(b.Ptr.Add(slots, b.I64.Const(int64(8*slot))))
		slot++
		switch t {
		case ir.TypeI1:
			return b.I32.Ne(b.I32.ULoad8(at), b.I32.Const(0))
		case ir.TypeI32:
			return b.I32.Load(at)
		case ir.TypeI64:
			return b.I64.Load(at)
		case ir.TypeF32:
			return b.F32.Load(at)
		case ir.TypeF64:
			return b.F64.Load(at)
		}
		return b.Ptr.Load(at)
	}
	var args []ir.Value
	for _, p := range params {
		if p.mapped {
			args = append(args, load(ir.TypePtr), load(ir.TypeI64))
		} else {
			args = append(args, load(p.reg))
		}
	}
	args = append(args, load(ir.TypePtr), load(ir.TypeI64))
	i32 := ir.NewSig().Ret(ir.TypeI32)
	item := b.Call(l.runtimeFunc("vertex_gpu_workitem_id_x", i32)).Value(0).(ir.I32)
	group := b.Call(l.runtimeFunc("vertex_gpu_workgroup_id_x", i32)).Value(0).(ir.I32)
	size := b.Call(l.runtimeFunc("vertex_gpu_workgroup_size_x", i32)).Value(0).(ir.I32)
	index := b.I64.Add(b.I64.Mul(b.I64.SExtI32(group), b.I64.SExtI32(size)), b.I64.SExtI32(item))
	l.mapBody(f, b, base, params, res, args, index)
	return f
}
