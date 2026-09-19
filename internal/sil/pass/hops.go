package pass

import (
	"strings"

	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
)

// Hops home that cannot move the task.
//
// A nonisolated async function runs on the task's home executor, so after
// every await the generator checks the task is still there and hops back
// if not: `vertex_task_needs_hop(home)`, then a branch to a hop. What was
// awaited decides whether the task can have moved. A nonisolated async
// function -- a Vertex one, called directly or through a closure whose
// type is not @MainActor -- ends on the task's home: it hopped there in its
// prologue if it awaits anything, checked the same way after each of its
// own awaits, and otherwise never left where its caller was, which was
// home. So the check after awaiting one is answered before it is asked.
//
// That is what this pass removes: a home check whose nearest suspension
// before it, walking back through single predecessors, is a call of such
// a function. A check after anything else -- a runtime primitive (they are
// the vertex_ functions), a @MainActor function, a hop, the function's
// entry -- is left alone. Each one removed is a runtime call, a thread
// slot lookup and a branch saved per await.
func elideHomeHops(f *sil.Func) {
	if f.IsDeclaration() || f.Type() == nil || !f.Type().Async {
		return
	}
	preds := map[*sil.Block][]*sil.Block{}
	for _, b := range f.Blocks() {
		if t := b.Term(); t != nil {
			for _, s := range t.Successors() {
				preds[s] = append(preds[s], b)
			}
		}
	}
	var dead []*sil.Block
	for _, b := range f.Blocks() {
		check, bit, move, join := homeCheck(b)
		if check == nil {
			continue
		}
		if !movedNothing(b, check, preds) {
			continue
		}
		// The branch takes the join; the hop's block has no other way in.
		term := b.Term()
		term.Rewrite(sil.Br, sil.Aux{Dest: join})
		if len(preds[move]) == 1 {
			dead = append(dead, move)
		}
		// The check's instructions, now unread.
		for _, in := range []*sil.Inst{bit, check} {
			if r := in.Result(); r != nil && len(r.Uses()) == 0 {
				b.Erase(in)
			}
		}
	}
	for _, m := range dead {
		// Its instructions go first, so that nothing is left in the use
		// lists of the values they read.
		insts := append([]*sil.Inst(nil), m.Insts()...)
		for i := len(insts) - 1; i >= 0; i-- {
			m.Erase(insts[i])
		}
		f.RemoveBlock(m)
	}
}

// homeCheck finds, at the end of b, the check the generator's ensure(home)
// emits:
//
//	%n = apply @vertex_task_needs_hop(2)
//	%bit = builtin "cmp_ne_Int64"(%n, 0)
//	cond_br %bit, move, join
//
// and answers its apply, its compare, and the two blocks.
func homeCheck(b *sil.Block) (check, bit *sil.Inst, move, join *sil.Block) {
	term := b.Term()
	if term == nil || term.Op() != sil.CondBr || len(term.Aux().Args) != 0 || len(term.Aux().ElseArgs) != 0 {
		return nil, nil, nil, nil
	}
	c := term.Args()[0].Inst()
	if c == nil || c.Op() != sil.BuiltinCall || c.Aux().Name != "cmp_ne_Int64" || c.Block() != b {
		return nil, nil, nil, nil
	}
	n := c.Args()[0].Inst()
	if n == nil || n.Op() != sil.Apply || n.Block() != b || calleeName(n) != "vertex_task_needs_hop" {
		return nil, nil, nil, nil
	}
	where := n.Args()[1].Inst()
	if where == nil || where.Op() != sil.IntegerLiteral || where.Aux().Int != 2 {
		return nil, nil, nil, nil
	}
	return n, c, term.Aux().Dest, term.Aux().Else
}

// movedNothing reports whether the nearest suspension before check is a
// call of a nonisolated Vertex async function.
func movedNothing(b *sil.Block, check *sil.Inst, preds map[*sil.Block][]*sil.Block) bool {
	insts := b.Insts()
	at := len(insts)
	for i, in := range insts {
		if in == check {
			at = i
		}
	}
	for steps := 0; steps < 64; steps++ {
		for i := at - 1; i >= 0; i-- {
			if moves, known := suspension(insts[i]); known {
				return !moves
			}
		}
		// Back into the one block that leads here, if there is one: a
		// try_apply's normal edge, a block a straight line branched to.
		ps := preds[b]
		if len(ps) != 1 {
			return false
		}
		b = ps[0]
		insts = b.Insts()
		at = len(insts)
	}
	return false
}

// suspension reports whether in may suspend (known) and, if so, whether
// the task can come back from it anywhere but home (moves).
func suspension(in *sil.Inst) (moves, known bool) {
	if in.Op() != sil.Apply && in.Op() != sil.TryApply {
		return false, false
	}
	callee := in.Args()[0]
	switch t := callee.Type().Formal().Underlying().(type) {
	case *sil.FuncType:
		if !t.Async {
			return false, false
		}
		name := calleeName(in)
		if name == "" || strings.HasPrefix(name, "vertex_") || t.Isolated {
			return true, true
		}
		return false, true
	case *types.Signature:
		if !t.Async {
			return false, false
		}
		return t.Isolated, true
	}
	return true, true
}

// calleeName is the function an apply calls directly, or "".
func calleeName(in *sil.Inst) string {
	if ref := in.Args()[0].Inst(); ref != nil && ref.Op() == sil.FunctionRef {
		return ref.Aux().Name
	}
	return ""
}
