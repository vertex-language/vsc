package pass

import (
	"github.com/vertex-language/vsc/vil"
)

// Boxes to stack slots: Swift's allocbox-to-stack, and mandatory for
// the reason Swift makes it mandatory.
//
// SILGen gives every `var` a box, because a box is what a variable
// needs in the general case — a closure may capture it, and then the
// variable has to outlive the scope it was written in and be
// reference counted like anything else that does. Most variables are
// not captured, and for those the box is a heap allocation and a pair
// of reference-count operations standing in for a stack slot.
//
// So SILGen writes the general form and this undoes it where it was
// not needed, which is what `swiftc -emit-sil` shows against
// `-emit-silgen` for the same function:
//
//	%2 = alloc_box ${ var Int }, var, name "n"        alloc_stack [var_decl] $Int, var, name "n"
//	%3 = begin_borrow [var_decl] %2                   (gone)
//	%4 = project_box %3, 0                            (gone; readers read the slot)
//	  ...                                               ...
//	end_borrow %3                                     (gone)
//	destroy_value %2                                  dealloc_stack
//
// It is a mandatory pass rather than an optimization because what is
// below it cannot execute a box: nothing lowers one, and nothing
// should have to. A box that survives here is a variable that really
// does escape, and that is a diagnostic from lowering rather than
// something quietly dropped.

// promoteBoxes rewrites every box in f that does not escape.
func promoteBoxes(f *vil.Func) {
	if f.IsDeclaration() {
		return
	}
	// Collected first and rewritten after, because rewriting edits
	// the block's instruction list underneath a walk of it.
	var boxes []*vil.Inst
	for _, b := range f.Blocks() {
		for _, in := range b.Insts() {
			if in.Op() == vil.AllocBox {
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

// boxUses is what reads a box, and whether every one of them is
// something a stack slot can do instead.
//
// The question this answers is escape, and it is answered by listing
// what does not escape rather than by listing what does. A box
// reaches a `partial_apply` when a closure captures it, is stored
// when it is put in a structure, and is returned when it outlives the
// call — and each of those is a use this does not recognise, so each
// leaves the box alone. That is the safe direction to be wrong in:
// an unpromoted box is refused by lowering, and a wrongly promoted
// one is a dangling pointer.
type boxRefs struct {
	borrows  []*vil.Inst // begin_borrow [var_decl] of the box
	projects []*vil.Inst // project_box, of the box or of a borrow
	ends     []*vil.Inst // end_borrow
	destroys []*vil.Inst // destroy_value of the box
}

func boxUses(alloc *vil.Inst) (boxRefs, bool) {
	var refs boxRefs
	box := alloc.Result()
	if box == nil {
		return refs, false
	}
	for _, use := range box.Uses() {
		switch use.Op() {
		case vil.ProjectBox:
			refs.projects = append(refs.projects, use)

		case vil.DestroyValue:
			refs.destroys = append(refs.destroys, use)

		case vil.BeginBorrow:
			borrow := use.Result()
			if borrow == nil {
				return refs, false
			}
			// A borrow is transparent here only if what reads it is
			// itself something the slot can answer.
			for _, bu := range borrow.Uses() {
				switch bu.Op() {
				case vil.ProjectBox:
					refs.projects = append(refs.projects, bu)
				case vil.EndBorrow:
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

// promote turns the box into a stack slot and every reference to what
// it held into a reference to the slot.
func promote(alloc *vil.Inst, refs boxRefs) {
	box := alloc.Result()
	elem := boxElem(box.Type())
	if elem == (vil.Type{}) {
		return
	}

	// The allocation becomes a stack slot, and the value every reader
	// already holds becomes the slot's address. alloc_box owns what
	// it allocated; an address owns nothing, which is what makes the
	// destroy below a deallocation rather than a release.
	aux := alloc.Aux()
	slotAux := vil.Aux{Type: elem, Name: aux.Name, Attrs: withAttr(aux.Attrs, "var_decl")}
	alloc.Reshape(vil.AllocStack, slotAux)
	box.SetType(elem.Address())
	box.SetOwnership(vil.None)

	// Everything that reached the storage through the box now reaches
	// it directly.
	for _, in := range refs.projects {
		if r := in.Result(); r != nil {
			vil.ReplaceAllUses(r, box)
		}
		erase(in)
	}
	for _, in := range refs.ends {
		erase(in)
	}
	for _, in := range refs.borrows {
		if r := in.Result(); r != nil {
			vil.ReplaceAllUses(r, box)
		}
		erase(in)
	}

	// Releasing the box becomes releasing the frame slot. A stack
	// discipline is what SIL asks for and what gen already emits: the
	// destroy is where the scope ended, which is where the slot stops
	// being live.
	for _, in := range refs.destroys {
		in.Rewrite(vil.DeallocStack, vil.Aux{}, box)
	}
}

// boxElem is what a box holds, or the zero Type if the value was not
// a box after all.
func boxElem(t vil.Type) vil.Type {
	b, ok := t.Formal().(*vil.BoxType)
	if !ok || b.Elem() == nil {
		return vil.Type{}
	}
	return vil.Object(b.Elem())
}

// withAttr adds an attribute if it is not already there, keeping the
// order the rest were written in.
func withAttr(attrs []string, name string) []string {
	for _, a := range attrs {
		if a == name {
			return attrs
		}
	}
	return append([]string{name}, attrs...)
}

func erase(in *vil.Inst) {
	if b := in.Block(); b != nil {
		b.Erase(in)
	}
}
