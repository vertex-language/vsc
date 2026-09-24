package pass

import (
	"github.com/vertex-language/vsc/internal/sil"
)

// Allocbox-to-stack promotion pass. Promotes uncaptured alloc_box allocations
// to alloc_stack, removing borrow and project instructions and replacing
// destroy_value with dealloc_stack.

// promoteBoxes rewrites every box in f that does not escape.
func promoteBoxes(f *sil.Func) {
	if f.IsDeclaration() {
		return
	}
	// Collect boxes first to avoid mutating during iteration.
	var boxes []*sil.Inst
	for _, b := range f.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() == sil.AllocBox {
				boxes = append(boxes, in)
			}
		}
	}
	for _, in := range boxes {
		if uses, ok := boxUses(in); ok {
			promote(in, uses)
		}
	}
}

// boxRefs records instructions reading a box that can be safely promoted to stack.
type boxRefs struct {
	borrows  []*sil.Inst // begin_borrow [var_decl]
	projects []*sil.Inst // project_box
	ends     []*sil.Inst // end_borrow
	destroys []*sil.Inst // destroy_value
}

func boxUses(alloc *sil.Inst) (boxRefs, bool) {
	var refs boxRefs
	box := alloc.Result()
	if box == nil {
		return refs, false
	}
	for _, use := range box.Uses() {
		switch use.Op() {
		case sil.ProjectBox:
			refs.projects = append(refs.projects, use)

		case sil.DestroyValue:
			refs.destroys = append(refs.destroys, use)

		case sil.BeginBorrow:
			borrow := use.Result()
			if borrow == nil {
				return refs, false
			}
			// Verify all uses of the borrow are valid for a stack slot.
			for _, bu := range borrow.Uses() {
				switch bu.Op() {
				case sil.ProjectBox:
					refs.projects = append(refs.projects, bu)
				case sil.EndBorrow:
					refs.ends = append(refs.ends, bu)
				default:
					return refs, false
				}
			}
			refs.borrows = append(refs.borrows, use)

		default:
			return refs, false
		}
	}
	return refs, true
}

// promote converts alloc_box into alloc_stack and redirects projected uses to the slot.
func promote(alloc *sil.Inst, refs boxRefs) {
	box := alloc.Result()
	elem := boxElem(box.Type())
	if elem == (sil.Type{}) {
		return
	}

	// Convert alloc_box to alloc_stack with address type.
	aux := alloc.Aux()
	slotAux := sil.Aux{Type: elem, Name: aux.Name, Attrs: withAttr(aux.Attrs, "var_decl")}
	alloc.Reshape(sil.AllocStack, slotAux)
	box.SetType(elem.Address())
	box.SetOwnership(sil.None)

	// Replace project_box, end_borrow, and begin_borrow uses.
	for _, in := range refs.projects {
		if r := in.Result(); r != nil {
			sil.ReplaceAllUses(r, box)
		}
		erase(in)
	}
	for _, in := range refs.ends {
		erase(in)
	}
	for _, in := range refs.borrows {
		if r := in.Result(); r != nil {
			sil.ReplaceAllUses(r, box)
		}
		erase(in)
	}

	// Convert destroy_value to dealloc_stack. Releasing a box destroyed
	// what it held, so a non-trivial variable's value is taken and
	// destroyed before the slot goes: one initialized where it was
	// declared, and one declared without a value -- `var w: T`, given one
	// later, perhaps on some paths only. That one is zeroed each time its
	// declaration runs (see lower's zeroSlot), and zeros hold nothing to let
	// go of, so destroying it is right whether or not it was assigned.
	// Without this every such variable leaked what it last held.
	destroy := !elem.Trivial() && (initializedAtDeclaration(alloc) || hasAttr(alloc, "zeroed"))
	for _, in := range refs.destroys {
		if destroy {
			b := in.Block()
			v := b.InsertBefore(in, sil.Load, sil.Aux{Attrs: []string{"take"}}, []*sil.Value{box}, elem.Object()).Result()
			b.InsertBefore(in, sil.DestroyValue, sil.Aux{}, []*sil.Value{v})
		}
		in.Rewrite(sil.DeallocStack, sil.Aux{}, box)
	}
}

// initializedAtDeclaration reports whether the slot is initialized in its allocation block.
func initializedAtDeclaration(alloc *sil.Inst) bool {
	slot := alloc.Result()
	for _, use := range slot.Uses() {
		if use.Op() != sil.Store || use.Block() != alloc.Block() {
			continue
		}
		if args := use.Args(); len(args) == 2 && args[1] == slot {
			for _, a := range use.Aux().Attrs {
				if a == "init" {
					return true
				}
			}
		}
	}
	return false
}

// boxElem returns the element type held by a box, or zero Type if not a box.
func boxElem(t sil.Type) sil.Type {
	b, ok := t.Formal().(*sil.BoxType)
	if !ok || b.Elem() == nil {
		return sil.Type{}
	}
	return sil.Object(b.Elem())
}

// withAttr prepends an attribute if not present.
func withAttr(attrs []string, name string) []string {
	for _, a := range attrs {
		if a == name {
			return attrs
		}
	}
	return append([]string{name}, attrs...)
}

func erase(in *sil.Inst) {
	if b := in.Block(); b != nil {
		b.Erase(in)
	}
}
