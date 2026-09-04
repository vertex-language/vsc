package gen

import (
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Type lowering: from what a program says a type is to what the IR
// has to move around.
//
// The formal type survives — a VIL type is the analyzer's type with
// an address bit — so the interesting part is the conventions, which
// are ownership crossing a call boundary. Swift's defaults, because
// they are the ones `swiftc -emit-silgen` prints:
//
//	trivial      no convention at all; there is nothing to own
//	borrowing    @guaranteed, and it is the default for a parameter
//	consuming    @owned
//	inout        @inout, and an address rather than a value
//	returning    @owned, since the caller receives what it must release

// lowerType is the VIL type of a formal type.
func lowerType(t types.Type) vil.Type {
	if t == nil {
		return vil.Object(types.Typ[types.Void])
	}
	return vil.Object(t)
}

// paramConvention is how a parameter is passed.
func paramConvention(p *types.Param, t vil.Type) vil.ParamConvention {
	switch p.Ownership {
	case types.InOut:
		return vil.ParamInout
	case types.Consuming:
		return vil.ParamOwned
	case types.Borrowing:
		return vil.ParamGuaranteed
	}
	if t.Trivial() {
		return vil.ParamUnowned
	}
	// Swift's default: a parameter is borrowed for the call, and the
	// caller keeps it alive across it.
	return vil.ParamGuaranteed
}

// resultConvention is how a result comes back. A caller receives
// something it owns and must release, unless there is nothing to own.
func resultConvention(t vil.Type) vil.ResultConvention {
	if t.Trivial() {
		return vil.ResultUnowned
	}
	return vil.ResultOwned
}

// addressType is the VIL type of a parameter passed as an address.
func addressType(t vil.Type) vil.Type { return t.Address() }
