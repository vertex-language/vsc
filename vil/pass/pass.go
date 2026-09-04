// Package pass transforms a VIL module into another VIL module.
//
// Everything here takes a module and gives the same module back,
// changed. Nothing in it knows about VIR, about a target, or about
// linking: a pass is a fact about the language, and the language is
// what VIL holds.
//
// # The pipeline
//
// Swift's order, because the order is load-bearing. A module arrives
// raw from vil/gen — ownership written down, nothing checked — and
// leaves canonical, meaning every mandatory pass has run and what
// remains is a program that compiles. Then the ownership form itself
// is lowered away, leaving retain and release, which is what a
// backend can be given.
//
//	raw ──▶ mandatory passes ──▶ canonical ──▶ LowerOwnership ──▶ lowered
//
// The distinction that keeps this honest: a pass here may reject a
// program, and an optimization may never. They are different
// packages for that reason, and vil/opt does not exist yet.
//
// # What is here
//
// LowerOwnership, which erases the ownership form. The diagnostic
// passes — definite initialization, exclusivity, move-only checking —
// come after it in the sequence rather than before, because the first
// thing this compiler needs is a program that runs, and only the
// eliminator is on that path.
package pass

import (
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/verify"
)

// Mandatory runs the passes a program must survive to be lowerable
// and moves the module to canonical.
//
// Today that is the verifier and nothing else: vil/gen emits
// ownership correct by construction rather than leaving it to be
// worked out, so there is no ARC insertion to do — only the checking
// that it was done right. The diagnostic passes join it here.
func Mandatory(m *vil.Module) error {
	if err := verify.Module(m); err != nil {
		return err
	}
	m.SetStage(vil.StageCanonical)
	return nil
}
