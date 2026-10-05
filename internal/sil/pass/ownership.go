package pass

import (
	"github.com/vertex-language/vsc/internal/sil"
)

// LowerOwnership lowers OSSA ownership instructions to reference counting:
//   - copy_value -> strong_retain
//   - destroy_value -> strong_release
//   - load [copy] -> load + strong_retain
//   - begin_borrow, move_value, mark_uninitialized -> forwarded / erased
//   - end_borrow, extend_lifetime, end_lifetime -> erased
//   - strips [ossa] attribute and resets value ownership to None.
func LowerOwnership(m *sil.Module) error {
	for _, f := range m.Funcs() {
		lowerFunc(f)
	}
	m.SetStage(sil.StageLowered)
	return nil
}

func lowerFunc(f *sil.Func) {
	if f.IsDeclaration() || !f.OSSA() {
		return
	}
	for _, b := range f.Blocks() {
		lowerBlock(b)
	}
	// Clear ownership for all values in the function.
	for _, v := range f.Values() {
		v.SetOwnership(sil.None)
	}
	f.ClearAttr("ossa")
}

// lowerBlock lowers ownership instructions within a single block.
func lowerBlock(b *sil.Block) {
	insts := make([]*sil.Inst, len(b.Insts()))
	copy(insts, b.Insts())

	for _, in := range insts {
		args := in.Args()
		switch in.Op() {
		// Replace copy_value with strong_retain and forward uses.
		case sil.CopyValue:
			if len(args) == 1 {
				sil.ReplaceAllUses(in.Result(), args[0])
				in.Rewrite(sil.StrongRetain, sil.Aux{}, args[0])
			}

		case sil.DestroyValue:
			if len(args) == 1 {
				in.Rewrite(sil.StrongRelease, sil.Aux{}, args[0])
			}

		// load [copy] lowers to a plain load followed by strong_retain.
		case sil.Load:
			if in.Result() != nil && hasAttr(in, "copy") {
				in.Reshape(sil.Load, sil.Aux{})
				b.InsertAfter(in, sil.StrongRetain, sil.Aux{},
					[]*sil.Value{in.Result()})
			}

		// Forward operand to uses and erase.
		case sil.BeginBorrow, sil.MoveValue, sil.MarkUninitialized:
			if len(args) == 1 {
				sil.ReplaceAllUses(in.Result(), args[0])
				b.Erase(in)
			}

		case sil.EndBorrow, sil.ExtendLifetime, sil.EndLifetime:
			b.Erase(in)
		}
	}
}

// hasAttr reports whether an instruction carries an attribute.
func hasAttr(in *sil.Inst, name string) bool {
	for _, a := range in.Aux().Attrs {
		if a == name {
			return true
		}
	}
	return false
}
