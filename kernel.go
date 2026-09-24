package vsc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/vertex-language/air/metallib"
	"github.com/vertex-language/ir"
	irtext "github.com/vertex-language/ir/text"
	airlower "github.com/vertex-language/ir/lower/air"
	"github.com/vertex-language/ir/lower/inline"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/internal/sil/gen"
	"github.com/vertex-language/vsc/lower"
	"github.com/vertex-language/vsc/token"
)

// The device compile (proposed_vertex_kernel.md §8): each kernel of a
// module, and everything it reaches, lowered for the GPU.
//
// A kernel is found in the module's lowered SIL by its mark
// (gen.AttrKernel). What it reaches is found by following its calls: a
// function of this module, a function of the built-in gpu module (whose
// SIL the compiler makes from its own copy of gpu's source), or a gpu
// intrinsic. Anything else -- the runtime, a heap, I/O -- cannot run on a
// device, and the kernel is refused with the chain of calls that reached
// it. This is §5's check, made where every call a kernel makes is known.
//
// The functions reached make a module of their own, which lower lowers
// for the device (lower.Options.Device). Each kernel is then inlined into
// its entry, its shared storage made (vertex_gpu_shared's size is a
// constant by then), and the whole lowered to AIR and written as the
// Metal library that goes in the kernel's descriptor. A kernel AIR
// cannot express -- a float64 on Apple's GPUs -- has no library, and the
// runtime runs it on the CPU instead, with a warning here saying so.

// compileKernels builds each kernel of m for the devices, returning what
// lower writes into each kernel's descriptor.
func compileKernels(m *sil.Module, target ir.Target) (map[string]*lower.KernelImage, []Diagnostic) {
	// Grid kernels, launched as they are; element kernels, launched as
	// the grid kernel Map writes for each choice of mapped parameters a
	// launch in this module makes (<f>$map<mask>, named by the launch's
	// descriptor builtin).
	type entry struct {
		name string
		k    *sil.Func
	}
	var entries []entry
	for _, f := range m.Funcs() {
		if f.HasAttr(gen.AttrKernel) && !f.IsDeclaration() && len(f.Type().Results) == 0 {
			entries = append(entries, entry{f.Name(), f})
		}
	}
	seen := map[string]bool{}
	for _, f := range m.Funcs() {
		for _, b := range f.Blocks() {
			for _, in := range b.Insts() {
				if in.Op() != sil.BuiltinCall || !strings.HasPrefix(in.Aux().Name, gen.KernelDescriptorBuiltin) {
					continue
				}
				name := strings.TrimPrefix(in.Aux().Name, gen.KernelDescriptorBuiltin)
				i := strings.LastIndex(name, gen.KernelMapSuffix)
				if i < 0 || seen[name] {
					continue
				}
				seen[name] = true
				if k := m.Lookup(name[:i]); k != nil && !k.IsDeclaration() {
					entries = append(entries, entry{name, k})
				}
			}
		}
	}
	if len(entries) == 0 {
		return nil, nil
	}
	gpuSIL, err := gpuModuleSIL(target)
	if err != nil {
		return nil, []Diagnostic{phaseError(fmt.Errorf("the built-in gpu module: %w", err))}
	}
	out := map[string]*lower.KernelImage{}
	var diags []Diagnostic
	for _, e := range entries {
		img, ds := compileKernel(e.name, e.k, m, gpuSIL)
		diags = append(diags, ds...)
		if img != nil {
			out[e.name] = img
		}
	}
	return out, diags
}

// compileKernel builds one kernel, as the entry name says: the kernel k
// itself, or a map variant of it.
func compileKernel(name string, k *sil.Func, m, gpuSIL *sil.Module) (*lower.KernelImage, []Diagnostic) {
	dev := sil.NewModule(m.Name()+"_device", sil.StageLowered)
	if err := reachDevice(k, m, gpuSIL, dev); err != nil {
		return nil, []Diagnostic{kernelError(k, err.Error())}
	}
	vir, err := lower.Module(dev, ir.AIR64, lower.Options{Device: true, DeviceKernels: []string{name}})
	if err != nil {
		return nil, []Diagnostic{kernelError(k, "cannot be lowered for a device: "+err.Error())}
	}
	entry := func(f *ir.Func) bool { return f.Signature().CallConv() == ir.Kernel }
	if err := inline.Module(vir, inline.Options{Into: entry}); err != nil {
		return nil, []Diagnostic{kernelError(k, "cannot be inlined for a device: "+err.Error())}
	}
	img := &lower.KernelImage{}
	for _, f := range vir.Funcs() {
		if !entry(f) {
			continue
		}
		foldPointerWords(f)
		if err := resolveShared(vir, f); err != nil {
			return nil, []Diagnostic{kernelError(k, err.Error())}
		}
		f.WalkInsts(func(in *ir.Inst) bool {
			if in.Op().Verb == ir.VBarrier {
				img.Barrier = true
			}
			return true
		})
	}
	if err := vir.Err(); err != nil {
		return nil, []Diagnostic{kernelError(k, "cannot be lowered for a device: "+err.Error())}
	}
	// VSC_DUMP_DEVICE=dir writes each kernel's device VIR there, as the
	// AIR lowering is handed it: for reading what a kernel became.
	if dir := os.Getenv("VSC_DUMP_DEVICE"); dir != "" {
		var buf strings.Builder
		if err := irtext.Print(&buf, vir); err == nil {
			_ = os.WriteFile(filepath.Join(dir, k.SourceName()+".vir"), []byte(buf.String()), 0o644)
		}
	}
	var diags []Diagnostic
	am, err := airlower.Lower(vir, airlower.Options{})
	if err == nil {
		img.Metallib, err = metallib.Build(am)
	}
	if dir := os.Getenv("VSC_DUMP_DEVICE"); dir != "" && err == nil {
		_ = os.WriteFile(filepath.Join(dir, k.SourceName()+".metallib"), img.Metallib, 0o644)
	}
	if err != nil {
		img.Metallib = nil
		d := kernelError(k, "is not built for Apple GPUs, and runs on the CPU there: "+err.Error())
		d.Severity = token.Warn
		diags = append(diags, d)
	}
	return img, diags
}

// kernelError is a diagnostic about a kernel, where no position is known
// better than the kernel's name.
func kernelError(k *sil.Func, msg string) Diagnostic {
	return Diagnostic{Diagnostic: token.Diagnostic{
		Severity: token.Error,
		Message:  "kernel '" + k.SourceName() + "' " + msg,
	}}
}

// reachDevice adds to dev every function k reaches, refusing one that
// cannot run on a device with the chain of calls that reached it.
func reachDevice(k *sil.Func, m, gpuSIL *sil.Module, dev *sil.Module) error {
	type visit struct {
		f     *sil.Func
		chain []string
	}
	seen := map[string]bool{}
	stack := []visit{{k, []string{k.SourceName()}}}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[v.f.Name()] {
			continue
		}
		seen[v.f.Name()] = true
		dev.Adopt(v.f)
		for _, b := range v.f.Blocks() {
			for _, in := range b.Insts() {
				switch in.Op() {
				case sil.FunctionRef:
				case sil.ClassMethod, sil.WitnessMethod:
					return fmt.Errorf("reaches a call through a table (%s), which a device does not have: %s",
						in.Op(), strings.Join(v.chain, " calls "))
				case sil.GlobalAddr:
					return fmt.Errorf("reads a global variable, which a device does not have: %s",
						strings.Join(v.chain, " calls "))
				case sil.AllocRef, sil.AllocBox:
					return fmt.Errorf("allocates on the heap, which a device does not have: %s",
						strings.Join(v.chain, " calls "))
				default:
					continue
				}
				name := in.Aux().Name
				if seen[name] {
					continue
				}
				callee := m.Lookup(name)
				if callee == nil || callee.IsDeclaration() {
					if g := gpuSIL.Lookup(name); g != nil && (callee == nil || !g.IsDeclaration()) {
						callee = g
					}
				}
				if callee == nil {
					return fmt.Errorf("calls %s, which this compiler cannot find", name)
				}
				chain := append(append([]string(nil), v.chain...), callee.SourceName())
				if callee.IsDeclaration() {
					if strings.HasPrefix(name, "vertex_gpu_") {
						dev.Adopt(callee)
						seen[name] = true
						continue
					}
					return fmt.Errorf("cannot run on a device: %s, which %s",
						strings.Join(chain, " calls "), whyNotOnDevice(name))
				}
				stack = append(stack, visit{callee, chain})
			}
		}
	}
	return nil
}

// whyNotOnDevice says what a function a kernel reaches needs that a
// device does not have.
func whyNotOnDevice(name string) string {
	switch {
	case strings.HasPrefix(name, "vertex_print"):
		return "prints, and a kernel cannot print"
	case strings.Contains(name, "array"), strings.Contains(name, "Array"):
		return "is an Array's, and an Array lives on a heap a device does not have"
	case strings.Contains(name, "string"), strings.Contains(name, "String"):
		return "is a String's, and a String lives on a heap a device does not have"
	case strings.HasPrefix(name, "vertex_fatal"):
		return "stops the program, which a kernel does by trapping"
	case strings.HasPrefix(name, "vertex_task"), strings.Contains(name, "async"):
		return "is async, and there is nothing on a device to resume it"
	case strings.HasPrefix(name, "vertex_"):
		return "is the runtime's, which a device does not have"
	}
	return "has no body a device can run"
}

// resolveShared makes each vertex_gpu_shared call in a kernel's entry the
// workgroup storage it asks for: a shared global of that many bytes. The
// size must be a constant by now; the kernel is inlined.
func resolveShared(m *ir.Module, f *ir.Func) error {
	var calls []*ir.Inst
	f.WalkInsts(func(in *ir.Inst) bool {
		if in.Op().Verb == ir.VCall {
			if c := in.Callee(); c != nil && c.Name() == "vertex_gpu_shared" {
				calls = append(calls, in)
			}
		}
		return true
	})
	for i, call := range calls {
		if call.NumArgs() < 2 {
			return fmt.Errorf("asks for shared storage in a way this compiler does not understand")
		}
		size, ok := foldConst(call.Arg(0), 0)
		if !ok || size <= 0 {
			return fmt.Errorf("asks for gpu.Shared storage of a size that is not a constant: " +
				"the count of a gpu.Shared must be known when the kernel is compiled")
		}
		align, ok := foldConst(call.Arg(1), 0)
		if !ok || align <= 0 {
			align = 16
		}
		g := m.Global(fmt.Sprintf("vertex_shared_%d", i), ir.Shared, ir.Array(uint64(size), ir.StoreI8.FType())).
			Align(uint64(align))
		blk := call.Block()
		rest := blk.SplitAfter(call, fmt.Sprintf("shared%d", i))
		addr := blk.Ptr.GetAddr(g)
		blk.Br(rest.To())
		if r := call.Result(0); r != nil {
			f.ReplaceUses(r, addr.Def())
		}
		blk.Remove(call)
	}
	return nil
}

// foldPointerWords makes a pointer that went through a word -- as a
// struct passed as words carries one, across a call and, once inlined,
// across the block parameters its return became -- the pointer again:
// ptr.from_i64 of a word that is i64.from_ptr p is p. A device has no
// generic pointer, and the space of p is what the AIR lowering infers
// from where it came from; through a word, it would come from nowhere.
func foldPointerWords(f *ir.Func) {
	f.WalkInsts(func(in *ir.Inst) bool {
		if in.Op().Verb != ir.VFromI64 || in.Op().Type != ir.TypePtr || in.NumArgs() != 1 {
			return true
		}
		src := wordOrigin(in.Arg(0), 0)
		if src == nil {
			return true
		}
		if r := in.Result(0); r != nil {
			f.ReplaceUses(r, src)
		}
		return true
	})
}

// wordOrigin is the pointer a word was made from by i64.from_ptr, looked
// for through block parameters every predecessor passes the same such
// word to; nil where it was made some other way.
func wordOrigin(d *ir.Def, depth int) *ir.Def {
	if d == nil || depth > 64 {
		return nil
	}
	if in := d.Inst(); in != nil {
		if in.Op().Verb == ir.VFromPtr && in.NumArgs() == 1 {
			return in.Arg(0)
		}
		return nil
	}
	blk := d.Block()
	if blk == nil {
		return nil
	}
	idx := -1
	for i, p := range blk.Params() {
		if p == d {
			idx = i
		}
	}
	if idx < 0 {
		return nil
	}
	var origin *ir.Def
	for _, pred := range blk.Preds() {
		term := pred.Term()
		if term == nil {
			return nil
		}
		for _, t := range term.Targets() {
			if t.Block() != blk {
				continue
			}
			if idx >= len(t.Args()) {
				return nil
			}
			o := wordOrigin(t.Args()[idx], depth+1)
			if o == nil || (origin != nil && o != origin) {
				return nil
			}
			origin = o
		}
	}
	return origin
}

// foldConst is the value of an integer the kernel computes from constants
// alone: constants, and sums, differences, products and shifts of them,
// through block parameters every predecessor passes the same one.
func foldConst(d *ir.Def, depth int) (int64, bool) {
	if d == nil || depth > 32 {
		return 0, false
	}
	in := d.Inst()
	if in == nil {
		// A block parameter: the value every predecessor passes.
		blk := d.Block()
		if blk == nil {
			return 0, false
		}
		idx := -1
		for i, p := range blk.Params() {
			if p == d {
				idx = i
			}
		}
		if idx < 0 {
			return 0, false
		}
		var val int64
		found := false
		for _, pred := range blk.Preds() {
			term := pred.Term()
			if term == nil {
				return 0, false
			}
			for _, t := range term.Targets() {
				if t.Block() != blk || idx >= len(t.Args()) {
					continue
				}
				v, ok := foldConst(t.Args()[idx], depth+1)
				if !ok || (found && v != val) {
					return 0, false
				}
				val, found = v, true
			}
		}
		return val, found
	}
	switch in.Op().Verb {
	case ir.VConst:
		c, ok := in.Lit()
		return c.Int(), ok
	case ir.VAdd, ir.VSub, ir.VMul, ir.VShl:
		a, ok1 := foldConst(in.Arg(0), depth+1)
		b, ok2 := foldConst(in.Arg(1), depth+1)
		if !ok1 || !ok2 {
			return 0, false
		}
		switch in.Op().Verb {
		case ir.VAdd:
			return a + b, true
		case ir.VSub:
			return a - b, true
		case ir.VMul:
			return a * b, true
		default:
			return a << uint(b), true
		}
	}
	return 0, false
}

// gpuModuleSIL is the built-in gpu module's lowered SIL, for the target:
// what a kernel reaches of gpu, which the device compile takes bodies
// from. Made once per target per process.
func gpuModuleSIL(target ir.Target) (*sil.Module, error) {
	gpuSILCache.Lock()
	defer gpuSILCache.Unlock()
	key := target.String()
	if m, ok := gpuSILCache.m[key]; ok {
		return m, nil
	}
	dir, err := GPUSourceDir()
	if err != nil {
		return nil, err
	}
	names, err := filepath.Glob(filepath.Join(dir, "*"+SourceExtension))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var srcs []Source
	for _, n := range names {
		text, err := os.ReadFile(n)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, Source{Name: n, Text: text})
	}
	u, diags := Compile(srcs, Options{Module: GPUModule, Target: target, Stop: Lowered})
	if Errors(diags) {
		var b strings.Builder
		for _, d := range diags {
			b.WriteString("\n  ")
			b.WriteString(d.String())
		}
		return nil, fmt.Errorf("does not compile:%s", b.String())
	}
	if gpuSILCache.m == nil {
		gpuSILCache.m = map[string]*sil.Module{}
	}
	gpuSILCache.m[key] = u.SIL
	return u.SIL, nil
}

var gpuSILCache struct {
	sync.Mutex
	m map[string]*sil.Module
}
