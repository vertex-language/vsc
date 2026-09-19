package pass

import (
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/internal/sil/verify"
)

// Mandatory runs required passes (box promotion, assign resolution)
// and transitions the module stage to canonical. It verifies the module
// before and after transformation.
func Mandatory(m *sil.Module) error {
	if err := verify.Module(m); err != nil {
		return err
	}
	for _, f := range m.Funcs() {
		// Erase DI initialization marks before promotion and assign resolution.
		eraseMarks(f)
		promoteBoxes(f)
		resolveAssigns(f)
	}
	if err := verify.Module(m); err != nil {
		return err
	}
	m.SetStage(sil.StageCanonical)
	return nil
}

// Optimize runs the passes that make a canonical module faster without
// changing what it means -- swiftc's performance pipeline, of which this is
// the first pass -- and verifies what they leave. Mandatory must have run.
func Optimize(m *sil.Module) error {
	for _, f := range m.Funcs() {
		elideHomeHops(f)
		promoteSlots(f)
	}
	return verify.Module(m)
}
