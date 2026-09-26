package pass

import (
	"os"
	"testing"

	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/internal/sil/verify"
)

// checkPasses says whether the module is verified again after each
// pipeline of passes, as well as once on the way in.
//
// The verification on the way in is what refuses a module SILGen got
// wrong, and always runs. The ones after are checks on the passes
// themselves -- that promotion and assign resolution kept the module's
// ownership sound -- which is the compiler testing itself: they run
// under `go test`, and with VSC_VERIFY=all, and not on every build, as
// swiftc verifies after each pass only in a debug build of itself.
var checkPasses = testing.Testing() || os.Getenv("VSC_VERIFY") == "all"

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
	if checkPasses {
		if err := verify.Module(m); err != nil {
			return err
		}
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
	if checkPasses {
		return verify.Module(m)
	}
	return nil
}
