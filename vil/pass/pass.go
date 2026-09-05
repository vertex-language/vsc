package pass

import (
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/verify"
)

// Mandatory runs the passes a program must survive to be lowerable
// and moves the module to canonical.
//
// Two of them, in the order Swift runs them. The verifier first,
// because a pass is entitled to assume the module it is handed is
// sound and there is no point transforming one that is not. Then
// allocbox-to-stack, which is mandatory rather than an optimization:
// vil/gen gives every `var` the box SILGen gives it, and nothing
// below here can execute a box. Definite initialization runs with it,
// for the same reason and as far as it can go without dataflow — see
// di.go for where it stops and why stopping there is the safe half.
//
// There is no ARC insertion to do, which is what a mandatory pass
// list mostly is elsewhere: vil/gen emits ownership correct by
// construction rather than leaving it to be worked out, so what is
// left is checking that it was. The diagnostic passes join here.
//
// The verifier runs again at the end, over what the passes produced.
// A pass that leaves a module unsound has to be caught by something,
// and the something cannot be the pass itself.
func Mandatory(m *vil.Module) error {
	if err := verify.Module(m); err != nil {
		return err
	}
	for _, f := range m.Funcs() {
		promoteBoxes(f)
		resolveAssigns(f)
	}
	if err := verify.Module(m); err != nil {
		return err
	}
	m.SetStage(vil.StageCanonical)
	return nil
}
