package pass

import (
	"os"
	"testing"

	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/internal/sil/verify"
)

// checkPasses controls whether passes re-verify modules after transformation.
// Enabled during testing or when VSC_VERIFY=all is set.
var checkPasses = testing.Testing() || os.Getenv("VSC_VERIFY") == "all"

// Mandatory runs required passes (box promotion, assign resolution)
// and transitions the module stage to canonical. It verifies the module
// before and after transformation.
func Mandatory(m *sil.Module) error {
	if err := verify.Module(m); err != nil {
		return err
	}
	return runMandatory(m)
}

// MandatoryUnverified runs mandatory passes without initial module verification,
// allowing inspection of malformed SIL (--emit sil with -skip-verify).
func MandatoryUnverified(m *sil.Module) error {
	return runMandatory(m)
}

func runMandatory(m *sil.Module) error {
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

// Optimize runs canonical optimization passes (eliding redundant hops, promoting stack slots).
// Mandatory must have run.
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
