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
	if isOptionalExistential(f.Type) {
		return g.weakExistentialLoad(addr, f)
	}
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
	if isOptionalExistential(v.Type().Formal()) {
		g.weakExistentialStore(addr, v)
		return
	}
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

// A weak property of a class-bound protocol's existential -- `weak var
// delegate: Delegate?` -- holds its object weakly beside the type and
// tables. The runtime reads and writes it through memory, a temporary
// holding the existential as a value would be.

// weakExistentialLoad reads the weak existential at addr: a strong one,
// owned by the statement, or nil.
func (g *gen) weakExistentialLoad(addr *sil.Value, f *types.Field) *sil.Value {
	t := lowerType(f.Type)
	g.weakExistentialCall(stdlib.WeakExistentialLoad)
	tmp := g.blk.AllocStack(t)
	access := g.blk.BeginAccess(addr, "read", "dynamic")
	g.blk.Apply(g.blk.FunctionRef(g.m.Func(stdlib.WeakExistentialLoad)), sil.Object(types.Typ[types.Void]),
		g.blk.AddressToPointer(access, rawPointerType()), g.blk.AddressToPointer(tmp, rawPointerType()),
		g.sizeLiteral(f.Type))
	g.blk.EndAccess(access)
	v := g.blk.Load(tmp, "take")
	return g.loaded(v, t)
}

// weakExistentialStore writes v, which the caller owns, into the weak
// existential at addr, and lets go of it.
func (g *gen) weakExistentialStore(addr, v *sil.Value) {
	g.weakExistentialCall(stdlib.WeakExistentialAssign)
	tmp := g.blk.AllocStack(v.Type())
	if v.Ownership() == sil.Owned {
		g.blk.Store(v, tmp, "init")
	} else {
		g.blk.Store(g.blk.CopyValue(v), tmp, "init")
	}
	access := g.blk.BeginAccess(addr, "modify", "dynamic")
	g.blk.Apply(g.blk.FunctionRef(g.m.Func(stdlib.WeakExistentialAssign)), sil.Object(types.Typ[types.Void]),
		g.blk.AddressToPointer(access, rawPointerType()), g.blk.AddressToPointer(tmp, rawPointerType()),
		g.sizeLiteral(v.Type().Formal()))
	g.blk.EndAccess(access)
	g.blk.DestroyAddr(tmp)
}

// weakExistentialCall declares the runtime's weak existential entry.
func (g *gen) weakExistentialCall(symbol string) {
	callee := g.m.Func(symbol).SetSourceName(symbol)
	if g.needsType(callee) {
		callee.SetLinkage(sil.PublicExternal)
		callee.Type().Convention = sil.Thin
		callee.Type().Params = []sil.Param{
			{Type: rawPointerType(), Convention: sil.ParamUnowned},
			{Type: rawPointerType(), Convention: sil.ParamUnowned},
			{Type: sil.Object(sil.BuiltinInt64), Convention: sil.ParamUnowned},
		}
		callee.SetResult(sil.Object(types.Typ[types.Void]), sil.ResultUnowned)
	}
}

// sizeLiteral is t's size in bytes, as a Builtin.Int64.
func (g *gen) sizeLiteral(t types.Type) *sil.Value {
	return g.blk.IntegerLiteral(sil.Object(sil.BuiltinInt64), types.Sizeof(t, types.DefaultTarget64))
}
