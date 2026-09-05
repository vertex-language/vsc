package pass

import (
	"github.com/vertex-language/vsc/vil"
)

// Definite initialization, as much of it as is provably right.
//
// `assign` is what SILGen writes for a store to a variable, and it
// means "put this here, and destroy whatever was here already". Which
// of those two halves is real depends on whether the destination had
// been initialized yet, and that is a question about every path
// reaching the store rather than about the store — which is why Swift
// answers it in a dataflow pass and why canonical SIL has no `assign`
// left in it.
//
// This does the half that needs no dataflow. A trivial type owns
// nothing, so there is nothing to destroy, so the assignment is a
// store whether the location was initialized or not — the answer is
// the same down every path and the analysis is not needed to find it.
// Int, Bool, and the floats are trivial, which is every type a `var`
// can hold in this compiler today.
//
// What is deliberately left alone is an assignment to a location
// holding something owned. Turning that into a store would leak what
// was there, and turning it into destroy-then-store would double-free
// where the location was not yet initialized. Both are wrong, and
// which one is wrong here is exactly what the dataflow would say — so
// the `assign` stays, and lowering refuses it by name rather than
// guessing. When there is a real definite-initialization pass, it
// replaces this one and this comment goes with it.

// resolveAssigns rewrites the assignments whose answer is not in
// doubt.
func resolveAssigns(f *vil.Func) {
	if f.IsDeclaration() {
		return
	}
	for _, b := range f.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() != vil.Assign {
				continue
			}
			// assign %value to %address.
			args := in.Args()
			if len(args) != 2 || args[0] == nil {
				continue
			}
			if !args[0].Type().Trivial() {
				continue
			}
			in.Reshape(vil.Store, vil.Aux{Attrs: []string{"trivial"}})
		}
	}
}
