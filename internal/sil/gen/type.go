package gen

import (
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Type and convention lowering for SIL operations.

// lowerType is the SIL type of a formal type.
func lowerType(t types.Type) sil.Type {
	if t == nil {
		return sil.Object(types.Typ[types.Void])
	}
	return sil.Object(t)
}

// paramConvention is how a parameter is passed.
func paramConvention(p *types.Param, t sil.Type) sil.ParamConvention {
	switch p.Ownership {
	case types.InOut:
		return sil.ParamInout
	case types.Consuming:
		return sil.ParamOwned
	case types.Borrowing:
		return sil.ParamGuaranteed
	}
	// Existentials are passed indirectly by address as @in_guaranteed.
	if isExistentialType(p.BodyType()) {
		return sil.ParamInGuaranteed
	}
	if t.Trivial() {
		return sil.ParamUnowned
	}
	// Default parameter convention is @guaranteed.
	return sil.ParamGuaranteed
}

// resultConvention is how a result comes back.
func resultConvention(t sil.Type) sil.ResultConvention {
	// An existential is returned as the value it is -- its words, which
	// lower hands back through storage the caller sets aside where they
	// are too many for registers.
	if t.Trivial() {
		return sil.ResultUnowned
	}
	return sil.ResultOwned
}

// isExistentialType reports whether t is an existential type.
func isExistentialType(t types.Type) bool {
	_, ok := existentialOf(t)
	return ok
}

// byAddress reports whether a parameter convention is passed indirectly by address.
func byAddress(c sil.ParamConvention) bool {
	return c == sil.ParamInGuaranteed || c == sil.ParamInout
}
