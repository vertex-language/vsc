package gen

import (
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/stdlib"
	"github.com/vertex-language/vsc/types"
)

// Weak and unowned properties of a class. Their word is a reference the
// runtime counts apart from the strong ones: it keeps the object's memory
// and not the object. A read asks the runtime for a strong reference --
// nil for a weak one once the object has ended, a trap for an unowned one
// -- and a write hands it the new object; the class's destroyer lets go of
// what the word holds as a weak reference (lower's classDestroyer).

// refSlot remembers that addr is a weak or unowned property's storage, so
// that an assignment to it writes through the runtime.
func (g *gen) refSlot(addr *sil.Value, f *types.Field) {
	if g.refSlots == nil {
		g.refSlots = map[*sil.Value]*types.Field{}
	}
	g.refSlots[addr] = f
}

// refLoad reads the weak or unowned property f held at addr: a strong
// reference, owned by the statement.
func (g *gen) refLoad(addr *sil.Value, f *types.Field) *sil.Value {
	symbol := stdlib.WeakLoad
	if f.Ref == "unowned" {
		symbol = stdlib.UnownedLoad
	}
	t := lowerType(f.Type)
	callee := g.m.Func(symbol).SetSourceName(symbol)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		callee.Type().Convention = sil.Thin
		callee.Type().Params = []sil.Param{{Type: rawPointerType(), Convention: sil.ParamUnowned}}
		callee.SetResult(t, sil.ResultOwned)
	}
	access := g.blk.BeginAccess(addr, "read", "dynamic")
	v := g.blk.Apply(g.blk.FunctionRef(callee), t, g.blk.AddressToPointer(access, rawPointerType()))
	g.blk.EndAccess(access)
	return g.loaded(v, t)
}

// refStore writes v, which the caller owns, into the weak or unowned
// property at addr, and lets go of it: the property keeps a weak
// reference of its own.
func (g *gen) refStore(addr, v *sil.Value) {
	callee := g.m.Func(stdlib.WeakAssign).SetSourceName(stdlib.WeakAssign)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		callee.Type().Convention = sil.Thin
		callee.Type().Params = []sil.Param{
			{Type: rawPointerType(), Convention: sil.ParamUnowned},
			{Type: v.Type(), Convention: sil.ParamGuaranteed},
		}
		callee.SetResult(sil.Object(types.Typ[types.Void]), sil.ResultUnowned)
	}
	access := g.blk.BeginAccess(addr, "modify", "dynamic")
	g.blk.Apply(g.blk.FunctionRef(callee), sil.Object(types.Typ[types.Void]),
		g.blk.AddressToPointer(access, rawPointerType()), v)
	g.blk.EndAccess(access)
	if v.Ownership() == sil.Owned {
		g.blk.DestroyValue(v)
	}
}
