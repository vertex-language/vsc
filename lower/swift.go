package lower

import (
	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// Swift ABI interop boundary for calling functions compiled with swiftc.
// Converts String and Array representations across calls using runtime bridging helpers.

// swiftCall reports whether a call's callee is a function swiftc built.
func (c *fn) swiftCall(callee *sil.Value) bool {
	name, ok := c.refNames[callee]
	if !ok {
		return false
	}
	f := c.l.module.Lookup(name)
	return f != nil && f.HasAttr(sil.AttrSwift)
}

// runtimeFunc imports a runtime entry point once per module.
func (l *lowerer) runtimeFunc(name string, sig *ir.Sig) ir.Callee {
	if f, ok := l.runtimeFns[name]; ok {
		return f
	}
	if l.runtimeFns == nil {
		l.runtimeFns = map[string]ir.Callee{}
	}
	f := l.out.ImportFunc(l.sym(name), sig).NoUnwind()
	l.runtimeFns[name] = f
	return f
}

// swiftBridged reports whether a type requires Swift ABI argument or result conversion.
func swiftBridged(t sil.Type) (isString, isArray bool, elem types.Type) {
	if isStringType(t) {
		return true, false, nil
	}
	if !t.IsValid() || t.IsAddress() || t.Formal() == nil {
		return false, false, nil
	}
	if a, ok := t.Formal().Underlying().(*types.Array); ok {
		return false, true, a.Elem
	}
	return false, false, nil
}

// bridgeableElements maps basic element types with identical binary representations in Vertex and Swift.
var bridgeableElements = map[types.BasicKind]string{
	types.Bool: "Bool", types.Int8: "Int8", types.UInt8: "UInt8", types.Int16: "Int16",
	types.UInt16: "UInt16", types.Int32: "Int32", types.UInt32: "UInt32", types.Float: "Float",
	types.Int: "Int", types.UInt: "UInt", types.Int64: "Int64", types.UInt64: "UInt64",
	types.Double: "Double",
}

// elementMetadata returns runtime metadata for bridgeable array elements.
func (c *fn) elementMetadata(in *sil.Inst, elem types.Type) (ir.Ptr, error) {
	b, ok := elem.Underlying().(*types.Basic)
	if ok {
		if name, known := bridgeableElements[b.Kind()]; known {
			g := c.l.metadataRecord(stdlib.Metadata(name))
			return c.b.Ptr.Add(c.b.Ptr.GetAddr(g), c.b.I64.Const(stdlib.MetadataOffset)), nil
		}
	}
	return ir.Ptr{}, c.fail(ErrUnsupported, in.Op(),
		"an array of '"+elem.String()+"' across the boundary with Swift, whose elements "+
			"are not the same bytes in both languages")
}

// bridgeArgToSwift converts argument registers to Swift representation, returning an optional post-call cleanup.
func (c *fn) bridgeArgToSwift(in *sil.Inst, a *sil.Value, regs []ir.Value,
	conv sil.ParamConvention) ([]ir.Value, func(), error) {
	isString, isArray, elem := swiftBridged(a.Type())
	owned := conv == sil.ParamOwned
	switch {
	case isString && len(regs) == 2:
		f := c.l.runtimeFunc(stdlib.SwiftStringToSwift,
			ir.NewSig().Param(ir.TypeI64).Param(ir.TypeI64).Ret(ir.TypeI64).Ret(ir.TypeI64))
		got := c.b.Call(f, regs[0], regs[1])
		out := []ir.Value{got.Value(0), got.Value(1)}
		if owned {
			// In owned convention, release the original Vertex string.
			release := c.l.runtimeFunc(stdlib.StringRelease, ir.NewSig().Param(ir.TypePtr))
			word, err := c.asPointer(in, regs[1])
			if err != nil {
				return nil, nil, err
			}
			return out, func() { c.b.Call(release, word) }, nil
		}
		release := c.l.runtimeFunc(stdlib.SwiftStringRelease, ir.NewSig().Param(ir.TypeI64))
		return out, func() { c.b.Call(release, out[1]) }, nil
	case isArray && len(regs) == 1:
		if _, err := c.elementMetadata(in, elem); err != nil {
			return nil, nil, err
		}
		p, ok := regs[0].(ir.Ptr)
		if !ok {
			return nil, nil, c.fail(ErrType, in.Op(), "an array that is not a reference")
		}
		f := c.l.runtimeFunc(stdlib.SwiftArrayToSwift, ir.NewSig().Param(ir.TypePtr).Ret(ir.TypePtr))
		swift := c.b.Call(f, p).Value(0)
		if owned {
			release := c.l.runtimeFunc(stdlib.Release, ir.NewSig().Param(ir.TypePtr))
			return []ir.Value{swift}, func() { c.b.Call(release, p) }, nil
		}
		release := c.l.runtimeFunc(stdlib.SwiftRelease, ir.NewSig().Param(ir.TypePtr))
		return []ir.Value{swift}, func() { c.b.Call(release, swift) }, nil
	}
	return regs, nil, nil
}

// bridgeResultFromSwift converts Swift call return registers back to Vertex representation.
func (c *fn) bridgeResultFromSwift(in *sil.Inst, r *sil.Value, regs []ir.Value) ([]ir.Value, error) {
	isString, isArray, elem := swiftBridged(r.Type())
	switch {
	case isString && len(regs) == 2:
		f := c.l.runtimeFunc(stdlib.SwiftStringFromSwift,
			ir.NewSig().Param(ir.TypeI64).Param(ir.TypeI64).Ret(ir.TypeI64).Ret(ir.TypeI64))
		got := c.b.Call(f, regs[0], regs[1])
		release := c.l.runtimeFunc(stdlib.SwiftStringRelease, ir.NewSig().Param(ir.TypeI64))
		c.b.Call(release, regs[1])
		return []ir.Value{got.Value(0), got.Value(1)}, nil
	case isArray && len(regs) == 1:
		meta, err := c.elementMetadata(in, elem)
		if err != nil {
			return nil, err
		}
		f := c.l.runtimeFunc(stdlib.SwiftArrayFromSwift,
			ir.NewSig().Param(ir.TypePtr).Param(ir.TypePtr).Ret(ir.TypePtr))
		return []ir.Value{c.b.Call(f, regs[0], meta).Value(0)}, nil
	}
	return regs, nil
}
