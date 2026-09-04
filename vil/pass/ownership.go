package pass

import (
	"github.com/vertex-language/vsc/vil"
)

// LowerOwnership erases the ownership form.
//
// Above this pass a value is owned or borrowed and the verifier can
// prove what happens to it. Below it there are only references and a
// count, which is what a machine can execute and what VIR can hold —
// VIR has no notion of ownership, and it should not: it is shared
// with compilers for languages that have none.
//
// The translation, which is Swift's:
//
//	copy_value %x        strong_retain %x, and %x wherever the copy was read
//	destroy_value %x     strong_release %x
//	begin_borrow %x      erased; readers read %x
//	end_borrow           erased
//	move_value %x        erased; readers read %x
//	extend_lifetime      erased
//	bb0(%0 : @owned $T)  bb0(%0 : $T)
//	sil [ossa] @f        sil @f
//
// What is not erased: the conventions in the function's own type. A
// parameter is still @guaranteed or @owned after this runs, because
// that is the contract between caller and callee rather than a fact
// about a value inside one — it is ABI, and the backend needs it.
//
// Access scopes are not ownership either. begin_access and end_access
// survive: they are where exclusivity is enforced, and enforcement is
// a runtime concern that outlives the ownership form.
func LowerOwnership(m *vil.Module) error {
	for _, f := range m.Funcs() {
		lowerFunc(f)
	}
	m.SetStage(vil.StageLowered)
	return nil
}

func lowerFunc(f *vil.Func) {
	if f.IsDeclaration() || !f.OSSA() {
		return
	}
	for _, b := range f.Blocks() {
		lowerBlock(b)
	}
	// Nothing inside owns anything any more, arguments included.
	for _, v := range f.Values() {
		v.SetOwnership(vil.None)
	}
	f.ClearAttr("ossa")
}

// lowerBlock rewrites one block. The instruction list is walked into
// a slice first: erasing while iterating over what is being erased
// from is how a pass loses instructions.
func lowerBlock(b *vil.Block) {
	insts := make([]*vil.Inst, len(b.Insts()))
	copy(insts, b.Insts())

	for _, in := range insts {
		args := in.Args()
		switch in.Op() {
		// A copy becomes a retain of what was copied, and everything
		// that read the copy reads the original: after this pass they
		// are the same reference, and the count is what keeps it
		// alive.
		case vil.CopyValue:
			if len(args) == 1 {
				vil.ReplaceAllUses(in.Result(), args[0])
				in.Rewrite(vil.StrongRetain, vil.Aux{}, args[0])
			}

		case vil.DestroyValue:
			if len(args) == 1 {
				in.Rewrite(vil.StrongRelease, vil.Aux{}, args[0])
			}

		// A borrow was a promise about lifetimes, and the promise has
		// been checked. Nothing is left to emit.
		case vil.BeginBorrow, vil.MoveValue, vil.MarkUninitialized:
			if len(args) == 1 {
				vil.ReplaceAllUses(in.Result(), args[0])
				b.Erase(in)
			}

		case vil.EndBorrow, vil.ExtendLifetime, vil.EndLifetime:
			b.Erase(in)
		}
	}
}
