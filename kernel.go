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
	airlower "github.com/vertex-language/ir/lower/air"
	"github.com/vertex-language/ir/lower/inline"
	ptxlower "github.com/vertex-language/ir/lower/ptx"
	irtext "github.com/vertex-language/ir/text"
	"github.com/vertex-language/ptx"
	ptxtext "github.com/vertex-language/ptx/text"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/internal/sil/gen"
	"github.com/vertex-language/vsc/lower"
	"github.com/vertex-language/vsc/token"
)

// GPU device compilation:
// Extracts kernel functions marked with gen.AttrKernel and follows the call graph
// across the module, the built-in gpu module, and imported @inlinable functions.
// Prohibited features (heap allocation, I/O, runtime calls) produce diagnostic errors.
// Reached functions are lowered into a device module, inlined, and translated to
// Apple Metal AIR or PTX. Kernels unsupported on device fall back to CPU execution.

// compileKernels builds each kernel of m for the devices, returning what
// lower writes into each kernel's descriptor.
func compileKernels(m *sil.Module, target ir.Target) (map[string]*lower.KernelImage, []Diagnostic) {
	// Collect grid kernels and synthesized element-kernel Map wrappers.
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
		img, ds := compileKernel(e.name, e.k, m, gpuSIL, target)
		diags = append(diags, ds...)
		if img != nil {
			out[e.name] = img
		}
	}
	return out, diags
}

// compileKernel builds one kernel, as the entry name says: the kernel k
// itself, or a map variant of it -- for Apple's GPUs where the target is
// Apple's, and for NVIDIA's everywhere else.
func compileKernel(name string, k *sil.Func, m, gpuSIL *sil.Module, target ir.Target) (*lower.KernelImage, []Diagnostic) {
	dev := sil.NewModule(m.Name()+"_device", sil.StageLowered)
	if err := reachDevice(k, m, gpuSIL, dev); err != nil {
		return nil, []Diagnostic{kernelError(k, err.Error())}
	}
	img := &lower.KernelImage{}
	if appleTarget(target) {
		vir, ds := deviceVIR(name, k, dev, ir.AIR64, false, img)
		if vir == nil {
			return nil, ds
		}
		// f16 and bf16 in f32 until the AIR lowering selects Metal's own
		// half instructions (see ir.Module.LegalizeHalf) -- but for
		// widening an f16, which the GPU does in one instruction where the
		// legalized form is a dozen: a quantized weight's scale is read
		// that way in every block.
		vir.LegalizeHalfNativeWiden()
		if err := vir.Err(); err != nil {
			return nil, []Diagnostic{kernelError(k, "cannot be lowered for a device: "+err.Error())}
		}
		dumpDevice(k, ".vir", func() []byte { return printVIR(vir) })
		am, err := airlower.Lower(vir, airlower.Options{})
		if err == nil {
			img.Metallib, err = metallib.Build(am)
		}
		if err != nil {
			img.Metallib = nil
			return img, []Diagnostic{kernelWarning(k, "is not built for Apple GPUs, and runs on the CPU there: "+err.Error())}
		}
		dumpDevice(k, ".metallib", func() []byte { return img.Metallib })
		return img, nil
	}
	// A CUDA launch runs whole blocks, past the grid's end: the entry
	// takes the grid's size and returns from a work-item outside it.
	vir, ds := deviceVIR(name, k, dev, ir.NVPTX64, true, img)
	if vir == nil {
		return nil, ds
	}
	// f16 and bf16 in f32: the PTX lowering selects no half instructions.
	vir.LegalizeHalf()
	if err := vir.Err(); err != nil {
		return nil, []Diagnostic{kernelError(k, "cannot be lowered for a device: "+err.Error())}
	}
	dumpDevice(k, ".vir", func() []byte { return printVIR(vir) })
	// sm_70 and PTX 8.0: what every CUDA 12 and later driver JITs, for
	// any GPU since Volta.
	pm, err := ptxlower.Lower(vir, ptxlower.Options{SM: ptx.SM70, ISA: ptx.ISA80, Prune: true})
	var src string
	if err == nil {
		src, err = ptxtext.Print(pm)
	}
	if err != nil {
		return img, []Diagnostic{kernelWarning(k, "is not built for NVIDIA GPUs, and runs on the CPU there: "+err.Error())}
	}
	img.PTX = []byte(src)
	dumpDevice(k, ".ptx", func() []byte { return img.PTX })
	return img, nil
}

// deviceVIR lowers the device module dev for a GPU target, with name's
// entry, inlined into it, its shared storage made; img.Barrier says
// whether it waits at one. Nil and why, where it cannot be.
func deviceVIR(name string, k *sil.Func, dev *sil.Module, target ir.Target, guard bool, img *lower.KernelImage) (*ir.Module, []Diagnostic) {
	vir, err := lower.Module(dev, target, lower.Options{Device: true, DeviceKernels: []string{name}, GridGuard: guard})
	if err != nil {
		return nil, []Diagnostic{kernelError(k, "cannot be lowered for a device: "+err.Error())}
	}
	entry := func(f *ir.Func) bool { return f.Signature().CallConv() == ir.Kernel }
	if err := inline.Module(vir, inline.Options{Into: entry}); err != nil {
		return nil, []Diagnostic{kernelError(k, "cannot be inlined for a device: "+err.Error())}
	}
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
	return vir, nil
}

// appleTarget is whether a target's GPUs are Apple's, which Metal drives.
func appleTarget(t ir.Target) bool {
	return strings.HasSuffix(t.Use(), "/macos") || strings.HasSuffix(t.Use(), "/ios")
}

// dumpDevice writes what a kernel became, for reading, where
// VSC_DUMP_DEVICE names a directory: its device VIR as a backend is handed
// it, and the backend's image.
func dumpDevice(k *sil.Func, ext string, bytes func() []byte) {
	if dir := os.Getenv("VSC_DUMP_DEVICE"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, k.SourceName()+ext), bytes(), 0o644)
	}
}

func printVIR(m *ir.Module) []byte {
	var buf strings.Builder
	if err := irtext.Print(&buf, m); err != nil {
		return nil
	}
	return []byte(buf.String())
}

// kernelWarning is a kernelError that does not stop the build.
func kernelWarning(k *sil.Func, msg string) Diagnostic {
	d := kernelError(k, msg)
	d.Severity = token.Warn
	return d
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
					why := whyNotOnDevice(name)
					if strings.HasPrefix(name, "$s") && why == whyNotOnDevice("") {
						// Another module's function, whose body its
						// interface does not carry.
						why = "is another module's: its body is not in that module's interface. Mark it @inlinable there"
					}
					return fmt.Errorf("cannot run on a device: %s, which %s",
						strings.Join(chain, " calls "), why)
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
