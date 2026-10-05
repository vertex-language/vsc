package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/types"
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
	label := g.takeLabel()
	g.iterate(s.Seq, it, label, func(payload *sil.Value, wrapped sil.Type, elem types.Type, header, exit *sil.Block, depth int) {
		if s.Case.IsValid() {
			g.loopCase = &loopElement{value: payload, typ: elem}
		} else {
			g.bindLoopVar(s.Pat, payload, wrapped)
		}
		g.loops = append(g.loops, loop{header: header, exit: exit, depth: depth, label: label})
		g.forInBody(s)
		g.loops = g.loops[:len(g.loops)-1]
	})
}

// iterate lowers the loop over seq an iteration describes, running body
// for each element in a scope of the element's own, which it may leave
// for header (continue) or exit (break) as a loop's body does.
func (g *gen) iterate(seq ast.Expr, it *analyzer.Iteration, label string,
	body func(payload *sil.Value, wrapped sil.Type, elem types.Type, header, exit *sil.Block, depth int)) {
	// The iterator, from makeIterator() or the sequence itself.
	var iter *sil.Value
	if it.MakeIterator != nil {
		call := &ast.CallExpr{Span: ast.Span{Lo: seq.Pos(), Hi: seq.End()}, Fun: &ast.MemberExpr{Span: ast.Span{Lo: seq.Pos(), Hi: seq.End()}, X: seq}}
		iter = g.methodCall(call, g.requirementOn(it.MakeIterator, g.typeOf(seq)), func() *sil.Value { return g.expr(seq) })
	} else {
		iter = g.expr(seq)
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
	iterExpr := &ast.IdentExpr{Span: ast.Span{Lo: seq.Pos(), Hi: seq.End()}}
	g.info.Types[iterExpr] = it.Iterator
	nextCall := &ast.CallExpr{Span: ast.Span{Lo: seq.Pos(), Hi: seq.End()}, Fun: &ast.MemberExpr{Span: ast.Span{Lo: seq.Pos(), Hi: seq.End()}, X: iterExpr}}

	elem := g.substituted(it.Element)
	wrapped := lowerType(elem)
	own := sil.Unowned
	if !wrapped.Trivial() {
		own = sil.Owned
	}

	header, exit := g.fn.Block(), g.fn.Block()
	g.blk.Br(header)

	// Each pass asks the iterator for its next element.
	g.blk = header
	g.push()
	nextRef := g.requirementOn(it.Next, itType)
	next := g.methodCall(nextCall, nextRef, func() *sil.Value {
		if mutatingRef(nextRef) {
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
	bodyBlk := g.fn.Block()
	payload := bodyBlk.Arg(wrapped, own)
	g.blk.SwitchEnum(v,
		sil.Case{Member: optionalSome, Dest: bodyBlk},
		sil.Case{Member: optionalNone, Dest: exit})

	g.blk = bodyBlk
	depth := len(g.scopes)
	g.push()
	body(payload, wrapped, elem, header, exit, depth)
	if g.blk != nil && g.blk.Term() == nil {
		g.pop()
		g.blk.Br(header)
	} else {
		g.scopes = g.scopes[:len(g.scopes)-1]
	}
	g.blk = exit
}

// requirementOn is the method a loop over a sequence of a type parameter
// calls -- Sequence's makeIterator, IteratorProtocol's next -- as the
// specialization's type implements it, where the checker resolved it
// against the parameter.
func (g *gen) requirementOn(ref *analyzer.MethodRef, recv types.Type) *analyzer.MethodRef {
	if resolved, ok := g.witness(ref, g.substituted(recv)); ok {
		return resolved
	}
	return ref
}

// arrayOfSequence lowers `Array(s)` of a sequence the checker can iterate
// -- a String's Characters, a Sequence of the program's own -- as Swift's
// init does: each element appended, in order, to an array made empty.
func (g *gen) arrayOfSequence(e *ast.CallExpr, it *analyzer.Iteration) *sil.Value {
	t := g.typeOf(e)
	arr, ok := arrayOf(t)
	if !ok {
		g.refuse(e, "an array of a sequence whose element type is not known")
		return nil
	}
	empty := g.makeArray(e, t, arr.Elem, nil)
	if empty == nil {
		return nil
	}
	g.forget(empty)
	lt := lowerType(t)
	box := g.blk.AllocBox(lt, "$array", "var")
	borrow := g.blk.BeginBorrow(box, "var_decl")
	slot := g.blk.ProjectBox(borrow, 0, lt)
	g.destroyLater(box)
	g.endBorrowLater(borrow)
	g.blk.Store(empty, slot, storeQualifier(lt))
	// A variable of the loop's own stands for the array, so that append
	// writes through it as it would through a variable the program named.
	span := ast.Span{Lo: e.Pos(), Hi: e.End()}
	sym := analyzer.NewVar("$array", t, e.Pos(), false, types.DefaultOwnership)
	array := &ast.IdentExpr{Span: span, Name: &ast.Ident{Span: span}}
	g.info.Uses[array.Name] = sym
	g.info.Types[array] = t
	g.locals[sym] = &local{addr: slot, box: box, typ: lt}
	appendElem, ok := core.LowerCollectionMethod(t, "append", []string{""})
	if !ok {
		g.refuse(e, "an array of a sequence of '"+arr.Elem.String()+"'")
		return nil
	}
	g.iterate(e.Args.Args[0].X, it, "", func(payload *sil.Value, wrapped sil.Type, _ types.Type, _, _ *sil.Block, _ int) {
		v := payload
		if v.Ownership() != sil.Owned && !wrapped.Trivial() {
			v = g.blk.CopyValue(v)
		}
		g.collectionCall(e, appendElem, array, []collArg{{value: v}})
	})
	delete(g.locals, sym)
	return g.loaded(g.blk.Load(slot, loadQualifier(lt)), lt)
}
