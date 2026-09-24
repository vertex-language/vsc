package gen

import (
	"strconv"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// KernelDescriptorBuiltin is the builtin a launch's kernel descriptor is:
// its name is this prefix and the kernel's symbol, and lowering answers
// it with the address of the descriptor the compiler writes beside that
// kernel (vsc/lower, gpu.go).
const KernelDescriptorBuiltin = "vertex_gpu_kernel_descriptor:"

// KernelMapSuffix is what a map variant's name adds to its element
// kernel's, before the mask of mapped parameters.
const KernelMapSuffix = "$map"

// AttrKernel marks the SIL function of a `kernel` function.
const AttrKernel = "kernel"

// kernelDescriptor lowers the call the checker wrote for a launch's
// kernel (analyzer/kernel.go) as the descriptor of that kernel.
func (g *gen) kernelDescriptor(k *analyzer.FuncSymbol) *sil.Value {
	raw := &types.Pointer{}
	return g.blk.Builtin(KernelDescriptorBuiltin+g.symbol(k), lowerType(raw))
}

// kernelMapDescriptor is kernelDescriptor for `k.Map(...)`: the grid
// kernel the compiler writes for the element kernel k with the mapped
// parameters mask, which lowering names <k>$map<mask>.
func (g *gen) kernelMapDescriptor(k *analyzer.FuncSymbol, mask uint64) *sil.Value {
	raw := &types.Pointer{}
	return g.blk.Builtin(KernelDescriptorBuiltin+g.symbol(k)+KernelMapSuffix+strconv.FormatUint(mask, 10), lowerType(raw))
}
