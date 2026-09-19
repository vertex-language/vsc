package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
)

// A for-in over a sequence of the program's own is what Swift makes of
// it: an iterator made once, asked for its next element before each
// pass, and the loop over when it answers nil.
//
//	var it = s.makeIterator()
//	while let x = it.next() { body }
//
// The iterator is a var of the loop's own, its next() a mutating call
// on that storage.

// forInIterator lowers a for-in the checker resolved to an iteration.
func (g *gen) forInIterator(s *ast.ForInStmt, it *analyzer.Iteration) {
	// The iterator, from makeIterator() or the sequence itself.
	var iter *sil.Value
	if it.MakeIterator != nil {
		call := &ast.CallExpr{Span: ast.Span{Lo: s.Seq.Pos(), Hi: s.Seq.End()}, Fun: &ast.MemberExpr{Span: ast.Span{Lo: s.Seq.Pos(), Hi: s.Seq.End()}, X: s.Seq}}
		iter = g.methodCall(call, it.MakeIterator, func() *sil.Value { return g.expr(s.Seq) })
	} else {
		iter = g.expr(s.Seq)
	}
	if iter == nil {
		return
	}
	iter = g.consume(iter)
	itType := g.substituted(it.Iterator)
	lt := lowerType(itType)
	box := g.blk.AllocBox(lt, "$iterator", "var")
	borrow := g.blk.BeginBorrow(box, "var_decl")
	slot := g.blk.ProjectBox(borrow, 0, lt)
	g.destroyLater(box)
	g.endBorrowLater(borrow)
	g.blk.Store(iter, slot, storeQualifier(lt))

	// A node standing for the iterator, for the call's receiver type.
	iterExpr := &ast.IdentExpr{Span: ast.Span{Lo: s.Seq.Pos(), Hi: s.Seq.End()}}
	g.info.Types[iterExpr] = it.Iterator
	nextCall := &ast.CallExpr{Span: ast.Span{Lo: s.Seq.Pos(), Hi: s.Seq.End()}, Fun: &ast.MemberExpr{Span: ast.Span{Lo: s.Seq.Pos(), Hi: s.Seq.End()}, X: iterExpr}}

	elem := g.substituted(it.Element)
	wrapped := lowerType(elem)
	own := sil.Unowned
	if !wrapped.Trivial() {
		own = sil.Owned
	}

	header, exit := g.fn.Block(), g.fn.Block()
	label := g.takeLabel()
	g.blk.Br(header)

	// Each pass asks the iterator for its next element.
	g.blk = header
	g.push()
	next := g.methodCall(nextCall, it.Next, func() *sil.Value {
		if mutatingRef(it.Next) {
			return slot
		}
		return g.loaded(g.blk.Load(slot, loadQualifier(lt)), lt)
	})
	if next == nil {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return
	}
	// The optional is the loop's to switch on; the temporaries the
	// call made end here, on both arms.
	v := g.consume(next)
	g.pop()
	body := g.fn.Block()
	payload := body.Arg(wrapped, own)
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: body},
		sil.Case{Member: optionalNone, Dest: exit})

	g.blk = body
	depth := len(g.scopes)
	g.push()
	if s.Case.IsValid() {
		g.loopCase = &loopElement{value: payload, typ: elem}
	} else {
		g.bindLoopVar(s.Pat, payload, wrapped)
	}
	g.loops = append(g.loops, loop{header: header, exit: exit, depth: depth, label: label})
	g.forInBody(s)
	g.loops = g.loops[:len(g.loops)-1]
	if g.blk != nil && g.blk.Term() == nil {
		g.pop()
		g.blk.Br(header)
	} else {
		g.scopes = g.scopes[:len(g.scopes)-1]
	}
	g.blk = exit
}
