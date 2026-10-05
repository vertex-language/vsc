package pass

import (
	"github.com/vertex-language/vsc/internal/sil"
)

// Definite initialization lowering. Lowers assign instructions to store [trivial]
// for trivial types, or load [take] + store [init] + destroy_value for non-trivial types.

// eraseMarks removes mark_uninitialized instructions, replacing uses with the underlying storage.
func eraseMarks(f *sil.Func) {
	if f.IsDeclaration() {
		return
	}
	for _, b := range f.Blocks() {
		insts := make([]*sil.Inst, len(b.Insts()))
		copy(insts, b.Insts())
		for _, in := range insts {
			if in.Op() != sil.MarkUninitialized {
				continue
			}
			args := in.Args()
			if len(args) != 1 || in.Result() == nil {
				continue
			}
			sil.ReplaceAllUses(in.Result(), args[0])
			b.Erase(in)
		}
	}
}

// resolveAssigns rewrites assign instructions into stores and destructions.
func resolveAssigns(f *sil.Func) {
	if f.IsDeclaration() {
		return
	}
	for _, b := range f.Blocks() {
		// Inserting shifts b.Insts() in place, so walk a copy.
		insts := make([]*sil.Inst, len(b.Insts()))
		copy(insts, b.Insts())
		for _, in := range insts {
			if in.Op() != sil.Assign {
				continue
			}
			// assign %value to %address.
			args := in.Args()
			if len(args) != 2 || args[0] == nil {
				continue
			}
			if args[0].Type().Trivial() {
				in.Reshape(sil.Store, sil.Aux{Attrs: []string{"trivial"}})
				continue
			}
			// Load old value [take], store new value [init], then destroy old value.
			// Store occurs before destroy to avoid premature deallocation in self-assignments.
			addr := args[1]
			old := b.InsertBefore(in, sil.Load, sil.Aux{Attrs: []string{"take"}},
				[]*sil.Value{addr}, args[0].Type())
			in.Reshape(sil.Store, sil.Aux{Attrs: []string{"init"}})
			b.InsertAfter(in, sil.DestroyValue, sil.Aux{}, []*sil.Value{old.Result()})
		}
	}
}
