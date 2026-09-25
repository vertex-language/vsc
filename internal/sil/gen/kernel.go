package gen

import (
	"strconv"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
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
//
// A generic kernel's launch names a specialization of it, which is lowered
// here, as any generic function is where it is called: a kernel of its
// own, whose descriptor the launch takes.
func (g *gen) kernelDescriptor(call *ast.CallExpr, k *analyzer.FuncSymbol) *sil.Value {
	raw := &types.Pointer{}
	spec, generic := g.info.Specializations[call]
	if !generic || len(spec.Args) == 0 {
		return g.blk.Builtin(KernelDescriptorBuiltin+g.symbol(k), lowerType(raw))
	}
	// Inside a specialization, the launch's type arguments may be the
	// enclosing function's type parameters: the specialization's types.
	if len(g.subst) > 0 {
		args := make([]types.Type, len(spec.Args))
		for i, a := range spec.Args {
			args[i] = types.Substitute(a, g.subst)
		}
		spec = analyzer.Specialization{Params: spec.Params, Args: args}
	}
	var name string
	{
		subst := spec.Subst()
		sig, isSig := types.Substitute(k.Signature(), subst).(*types.Signature)
		if !isSig {
			g.refuse(call, "a generic kernel whose signature this compiler cannot substitute")
			return nil
		}
		name = g.specializedSymbol(k, sig, spec)
		if err := g.emitSpecialization(k, name, subst); err != nil {
			g.errorAt(call, err.Error())
			return nil
		}
	}
	return g.blk.Builtin(KernelDescriptorBuiltin+name, lowerType(raw))
}

// kernelMapDescriptor is kernelDescriptor for `k.Map(...)`: the grid
// kernel the compiler writes for the element kernel k with the mapped
// parameters mask, which lowering names <k>$map<mask>.
func (g *gen) kernelMapDescriptor(k *analyzer.FuncSymbol, mask uint64) *sil.Value {
	raw := &types.Pointer{}
	return g.blk.Builtin(KernelDescriptorBuiltin+g.symbol(k)+KernelMapSuffix+strconv.FormatUint(mask, 10), lowerType(raw))
}
