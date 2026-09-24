package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// A matcher matches one pattern against a value, however the pattern is
// nested: `.node(.leaf, let n)`, `(true?, _)`, `.c(let i)?`. Where the
// value matches, the lowering goes on in the block the match ends in
// with every name the pattern has bound; where it does not, it goes to
// miss, letting go of whatever the match had taken apart on the way.
//
// It is the general case. A switch whose patterns are each one case of
// an enum goes through one switch_enum instead (see switchOnEnum), which
// is what swiftc's decision tree makes of it; this tries each pattern in
// turn, which is what swiftc makes of the rest.
type matcher struct {
	g *gen
	// miss is where a value that does not match goes, with nothing of
	// the match still owned.
	miss func() *sil.Block
	// owned is what the match has taken apart and owns: a payload out
	// of an enum, the parts of a tuple. It is let go of on the way to
	// miss, and by whoever owns the match once it succeeds.
	owned []*sil.Value
}

// fail is where a mismatch goes from here: miss, after what is owned so
// far is let go of.
func (m *matcher) fail() *sil.Block {
	if len(m.owned) == 0 {
		return m.miss()
	}
	b := m.g.fn.Block()
	for i := len(m.owned) - 1; i >= 0; i-- {
		b.DestroyValue(m.owned[i])
	}
	b.Br(m.miss())
	return b
}

// consumable is v as something a switch can take: v itself where it is
// trivial, a copy otherwise -- the value is the caller's to keep.
func (m *matcher) consumable(v *sil.Value) *sil.Value {
	if v.Type().Trivial() {
		return v
	}
	return m.g.blk.CopyValue(v)
}

// take notes a value the match now owns.
func (m *matcher) take(v *sil.Value) {
	if v.Ownership() == sil.Owned && !v.Type().Trivial() {
		m.owned = append(m.owned, v)
	}
}

// match matches p against v, of type t, and reports whether it could be
// lowered. Under var, names are bound to storage of their own.
func (m *matcher) match(p ast.Pattern, v *sil.Value, t types.Type) bool {
	return m.matchAs(p, v, t, token.LET)
}

func (m *matcher) matchAs(p ast.Pattern, v *sil.Value, t types.Type, kind token.Kind) bool {
	g := m.g
	switch pat := p.(type) {
	case nil, *ast.WildcardPattern:
		return true

	case *ast.ValueBindingPattern:
		return m.matchAs(pat.Pat, v, t, pat.Kind)

	case *ast.IdentPattern:
		return m.bind(pat, pat.Name, v, t, kind)

	case *ast.TypedPattern:
		return m.matchAs(pat.Pat, v, t, kind)

	case *ast.ExprPattern:
		// A name under a let -- `case let (a, b)` -- is bound; anything
		// else is a value the subject is compared with.
		if ie, ok := pat.X.(*ast.IdentExpr); ok && ie.Name != nil && g.info.Defs[ie.Name] != nil {
			return m.bind(pat, ie.Name, v, t, kind)
		}
		if isNilLiteral(pat.X) {
			if o, ok := optionalOf(t); ok {
				return m.matchOptional(pat, nil, false, v, o, kind)
			}
		}
		return m.compare(pat, v, t)

	// `case is T`: whether the value is a T at run time.
	case *ast.IsPattern:
		to := g.info.PatternTypes[pat]
		out, bit, ok := g.castValue(p, v, t, to)
		if !ok {
			return false
		}
		yes := m.castBranch(bit, out)
		g.blk = yes
		lt := lowerType(to)
		if !lt.Trivial() {
			g.blk.DestroyValue(g.blk.Load(out, "take"))
		}
		g.blk.DeallocStack(out)
		return true

	// `case let d as Double`: the value as a T, where it is one, matched
	// against the pattern before `as`.
	case *ast.AsPattern:
		to := g.info.PatternTypes[pat]
		if _, isEx := existentialOf(to); isEx {
			g.refuse(p, "an 'as' pattern to an existential")
			return false
		}
		out, bit, ok := g.castValue(p, v, t, to)
		if !ok {
			return false
		}
		g.blk = m.castBranch(bit, out)
		lt := lowerType(to)
		cast := g.blk.Load(out, loadQualifierTake(lt))
		g.blk.DeallocStack(out)
		m.take(cast)
		return m.matchAs(pat.Pat, cast, to, kind)

	case *ast.TuplePattern:
		tu, isTuple := t.Underlying().(*types.Tuple)
		if !isTuple || len(tu.Elements) != len(pat.Elems) {
			g.refuse(p, "a tuple pattern over "+t.String())
			return false
		}
		parts := m.parts(v, tu)
		for i, el := range pat.Elems {
			if !m.matchAs(el.Pat, parts[i], tu.Elements[i].Type, kind) {
				return false
			}
		}
		return true

	case *ast.OptionalPattern:
		o, ok := optionalOf(t)
		if !ok {
			g.refuse(p, "an optional pattern over "+t.String())
			return false
		}
		return m.matchOptional(pat, pat.Pat, true, v, o, kind)

	case *ast.EnumCasePattern:
		if pat.Name == nil {
			return false
		}
		name := g.text(pat.Name)
		if o, ok := optionalOf(t); ok && pat.Type == nil && (name == "some" || name == "none") {
			var inner ast.Pattern
			if pat.Args != nil && len(pat.Args.Elems) == 1 {
				inner = pat.Args.Elems[0].Pat
			}
			return m.matchOptional(pat, inner, name == "some", v, o, kind)
		}
		if _, isEnum := underlyingEnum(t); !isEnum {
			// `case Code.refused:` names a static property, compared.
			return m.compare(pat, v, t)
		}
		return m.matchCase(pat, v, t, kind)
	}
	g.refuse(p, "this pattern")
	return false
}

// bind binds a name to v: to the value itself under let, and under var
// to storage of its own that starts as the value.
func (m *matcher) bind(p ast.Pattern, name *ast.Ident, v *sil.Value, t types.Type, kind token.Kind) bool {
	g := m.g
	sym := g.info.Defs[name]
	if sym == nil {
		g.refuse(p, "a name in a pattern this compiler did not resolve")
		return false
	}
	lt := lowerType(t)
	if kind != token.VAR {
		g.locals[sym] = &local{value: v, typ: lt}
		return true
	}
	if !lt.Trivial() {
		v = g.blk.CopyValue(v)
	}
	box := g.blk.AllocBox(lt, g.text(name), "var")
	addr := g.blk.ProjectBox(box, 0, lt)
	g.blk.Store(v, addr, storeQualifier(lt))
	g.locals[sym] = &local{addr: addr, box: box, typ: lt}
	m.owned = append(m.owned, box)
	return true
}

// compare tests v against the value an expression pattern names -- a
// literal, a range, a static property -- going to miss where they differ.
func (m *matcher) compare(p ast.Pattern, v *sil.Value, t types.Type) bool {
	g := m.g
	// What the comparison makes ends with it, before the branch.
	g.push()
	bit, ok := g.patternTest(p, v, t)
	if !ok {
		g.scopes = g.scopes[:len(g.scopes)-1]
		return false
	}
	g.pop()
	if bit == nil {
		return true
	}
	next := g.fn.Block()
	g.blk.CondBr(bit, next, nil, m.fail(), nil)
	g.blk = next
	return true
}

// castBranch goes on to the block it answers where bit says a cast
// matched, and to miss where it did not, letting go of the slot the cast
// wrote nothing to on the way.
func (m *matcher) castBranch(bit, slot *sil.Value) *sil.Block {
	g := m.g
	yes, no := g.fn.Block(), g.fn.Block()
	g.blk.CondBr(bit, yes, nil, no, nil)
	no.DeallocStack(slot)
	no.Br(m.fail())
	return yes
}

// parts is a tuple's elements: taken apart where the match owns the
// tuple, so that it owns each counted part instead, and read out of it
// otherwise.
func (m *matcher) parts(v *sil.Value, tu *types.Tuple) []*sil.Value {
	g := m.g
	elems := make([]sil.Type, len(tu.Elements))
	for i, el := range tu.Elements {
		elems[i] = lowerType(el.Type)
	}
	if n := len(m.owned); n > 0 && m.owned[n-1] == v {
		m.owned = m.owned[:n-1]
		parts := g.blk.DestructureTuple(v, elems...)
		for _, part := range parts {
			m.take(part)
		}
		return parts
	}
	parts := make([]*sil.Value, len(elems))
	for i, lt := range elems {
		parts[i] = g.blk.TupleExtract(v, i, lt)
	}
	return parts
}

// matchOptional matches an optional: `x?`, `.some(x)`, `.some`, `.none`
// and `nil`. Where the case is some, inner is matched against what it
// holds.
func (m *matcher) matchOptional(p ast.Pattern, inner ast.Pattern, some bool, v *sil.Value, o *types.Optional, kind token.Kind) bool {
	g := m.g
	wrapped := lowerType(o.Wrapped)
	own := sil.Unowned
	if !wrapped.Trivial() {
		own = sil.Owned
	}
	subject := m.consumable(v)
	next := g.fn.Block()
	if !some {
		// Lowering wants both cases named: some goes to miss, letting go
		// of what it holds.
		drop := g.fn.Block()
		payload := drop.Arg(wrapped, own)
		if own == sil.Owned {
			drop.DestroyValue(payload)
		}
		drop.Br(m.fail())
		g.blk.SwitchEnum(subject,
			sil.Case{Member: optionalSome, Dest: drop},
			sil.Case{Member: optionalNone, Dest: next})
		g.blk = next
		return true
	}
	payload := next.Arg(wrapped, own)
	g.blk.SwitchEnum(subject,
		sil.Case{Member: optionalSome, Dest: next},
		sil.Case{Member: optionalNone, Dest: m.fail()})
	g.blk = next
	m.take(payload)
	return m.matchAs(inner, payload, o.Wrapped, kind)
}

// matchCase matches one case of an enum, and its payload against the
// patterns written for it.
func (m *matcher) matchCase(pat *ast.EnumCasePattern, v *sil.Value, t types.Type, kind token.Kind) bool {
	g := m.g
	name := g.text(pat.Name)
	k := g.caseOf(t, name)
	if k == nil {
		g.refuse(pat, "a case '"+name+"' that "+t.String()+" does not have")
		return false
	}
	lt := lowerType(t)
	subject := m.consumable(v)
	next := g.fn.Block()
	// Every other case hands the value back, let go of on the way out.
	other := m.fail()
	if !lt.Trivial() {
		b := g.fn.Block()
		b.DestroyValue(b.Arg(lt, sil.Owned))
		b.Br(other)
		other = b
	}
	var payload *sil.Value
	if k.AssociatedType != nil {
		storage := lowerType(sil.CaseStorage(k))
		own := sil.Unowned
		if !storage.Trivial() {
			own = sil.Owned
		}
		payload = next.Arg(storage, own)
	}
	g.blk.SwitchEnum(subject,
		sil.Case{Member: memberName(t, name), Dest: next},
		sil.Case{Dest: other})
	g.blk = next
	if payload == nil {
		if pat.Args != nil && len(pat.Args.Elems) > 0 {
			g.refuse(pat, "a pattern binding a value on a case that carries none")
			return false
		}
		return true
	}
	m.take(payload)
	if k.Indirect {
		// An indirect case hands over its box; the match owns the box
		// and a copy of what it holds.
		inner := lowerType(k.AssociatedType)
		addr := g.blk.ProjectBox(payload, 0, inner)
		payload = g.blk.Load(addr, loadQualifier(inner))
		m.take(payload)
	}
	if pat.Args == nil {
		return true
	}
	assoc := k.AssociatedType
	if len(pat.Args.Elems) == 1 {
		return m.matchAs(pat.Args.Elems[0].Pat, payload, assoc, kind)
	}
	tu, ok := assoc.Underlying().(*types.Tuple)
	if !ok || len(tu.Elements) != len(pat.Args.Elems) {
		g.refuse(pat, "a pattern whose names do not match what the case carries")
		return false
	}
	parts := m.parts(payload, tu)
	for i, el := range pat.Args.Elems {
		if !m.matchAs(el.Pat, parts[i], tu.Elements[i].Type, kind) {
			return false
		}
	}
	return true
}

// simpleCasePattern reports whether a switch item is one case of an enum
// whose payload patterns bind or compare and go no deeper -- what
// switchOnEnum lowers through one switch_enum. Anything nested -- a case
// inside a payload, an optional pattern, a tuple -- is matched a pattern
// at a time instead.
func (g *gen) simpleCasePattern(p ast.Pattern) bool {
	if bind, ok := p.(*ast.ValueBindingPattern); ok {
		if bind.Kind == token.VAR {
			return false
		}
		p = bind.Pat
	}
	pat, ok := p.(*ast.EnumCasePattern)
	if !ok || pat.Name == nil {
		return false
	}
	if pat.Args == nil {
		return true
	}
	for _, el := range pat.Args.Elems {
		if !g.flatPattern(el.Pat) {
			return false
		}
	}
	return true
}

// flatPattern reports whether a pattern binds a name, matches anything,
// or compares with a value, and holds no pattern inside it.
func (g *gen) flatPattern(p ast.Pattern) bool {
	switch pat := p.(type) {
	case nil, *ast.WildcardPattern, *ast.IdentPattern:
		return true
	case *ast.ValueBindingPattern:
		// `var n` is storage of its own, which the matcher makes.
		return pat.Kind != token.VAR && g.flatPattern(pat.Pat)
	case *ast.ExprPattern:
		return !isNilLiteral(pat.X)
	}
	return false
}

// simpleOptionalPatterns reports whether every item of a switch over an
// optional is a case of Optional holding at most a name or a wildcard --
// what switchOnOptional lowers through one switch_enum.
func (g *gen) simpleOptionalPatterns(clauses []*ast.CaseClause) bool {
	for _, cs := range clauses {
		for _, item := range cs.Items {
			c, ok := g.optionalCaseOf(item.Pat)
			if !ok || (c.inner != nil && (g.refutable(c.inner) || bindsVar(c.inner))) {
				return false
			}
		}
	}
	return true
}

// switchOnPatterns emits a switch whose items are matched one at a time,
// in order, each going on to the next where it does not match: the
// general case of a switch, over any subject. The subject is borrowed
// for the whole switch, and each match owns what it takes apart until
// the body it leads to ends.
func (g *gen) switchOnPatterns(s *ast.SwitchStmt, subject *sil.Value, t types.Type,
	clauses []*ast.CaseClause, bodies []*sil.Block) bool {
	defaultClause := -1
	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			defaultClause = i
		}
	}
	for i, cs := range clauses {
		if cs.Kind == token.DEFAULT {
			continue
		}
		for _, item := range cs.Items {
			if len(cs.Items) > 1 && g.bindsNames(&ast.TuplePattern{Elems: []*ast.TuplePatternElem{{Pat: item.Pat}}}) {
				g.refuse(item.Pat, "a case of several patterns that binds names")
				return false
			}
			var missBlk *sil.Block
			miss := func() *sil.Block {
				if missBlk == nil {
					missBlk = g.fn.Block()
				}
				return missBlk
			}
			m := &matcher{g: g, miss: miss}
			if !m.match(item.Pat, subject, t) {
				return false
			}
			if item.Where != nil {
				g.push()
				cond := g.rvalue(item.Where.Cond)
				if cond == nil {
					g.scopes = g.scopes[:len(g.scopes)-1]
					return false
				}
				bit := g.machine(cond, types.Typ[types.Bool])
				if bit == nil {
					g.refuse(item.Where.Cond, "a where clause of "+g.typeOf(item.Where.Cond).String())
					g.scopes = g.scopes[:len(g.scopes)-1]
					return false
				}
				g.pop()
				holds := g.fn.Block()
				g.blk.CondBr(bit, holds, nil, m.fail(), nil)
				g.blk = holds
			}
			// The body owns what the match took, from here on.
			if len(m.owned) > 0 {
				if g.armOwned == nil {
					g.armOwned = map[*sil.Block][]*sil.Value{}
				}
				g.armOwned[bodies[i]] = append(g.armOwned[bodies[i]], m.owned...)
			}
			g.blk.Br(bodies[i])
			if missBlk == nil {
				// Nothing fails to match this one: the rest are never tried.
				return true
			}
			g.blk = missBlk
		}
	}
	if defaultClause >= 0 {
		g.blk.Br(bodies[defaultClause])
		return true
	}
	// Exhaustive, as the checker holds it: no value gets this far.
	g.blk.Unreachable()
	return true
}

// bindsVar reports whether a pattern binds a name under var.
func bindsVar(p ast.Pattern) bool {
	found := false
	ast.Inspect(p, func(n ast.Node) bool {
		if b, ok := n.(*ast.ValueBindingPattern); ok && b.Kind == token.VAR {
			found = true
		}
		return !found
	})
	return found
}
