package gen

import (
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// A body that produces a value and whose end can be reached is missing a
// return. swiftc finds it the same way: SILGen puts an `unreachable` where
// the body falls off its end, and DiagnoseUnreachable reports that
// instruction if a path from the entry still reaches it once branches on
// constants are folded -- which is what lets `while true { }` end a body.

// missingReturn ends the body between start and end (its braces) at the
// block it falls off in, reporting it at end if that block can be reached.
// A body something in it was already refused in says nothing more: what
// was lowered of it is not the program.
func (g *gen) missingReturn(start, end token.Pos, kind string, result types.Type) {
	if g.fn != nil && g.blk != nil && !g.refusedWithin(start, end) && reachable(g.fn, g.blk) {
		g.diags = append(g.diags, token.Diagnostic{
			Pos:      end,
			End:      end + 1,
			Severity: token.Error,
			Message:  "missing return in " + kind + " expected to return '" + result.String() + "'",
			File:     g.file,
		})
	}
	g.blk.Unreachable()
}

// refusedWithin reports whether an error was reported between start and end.
func (g *gen) refusedWithin(start, end token.Pos) bool {
	for _, d := range g.diags {
		if d.Severity == token.Error && d.Pos >= start && d.Pos <= end {
			return true
		}
	}
	return false
}

// reachable reports whether a path from f's entry reaches target, following
// only the edge a branch on a constant condition takes.
func reachable(f *sil.Func, target *sil.Block) bool {
	entry := f.Entry()
	if entry == nil {
		return false
	}
	seen := map[*sil.Block]bool{entry: true}
	work := []*sil.Block{entry}
	for len(work) > 0 {
		b := work[len(work)-1]
		work = work[:len(work)-1]
		if b == target {
			return true
		}
		t := b.Term()
		if t == nil {
			continue
		}
		succs := t.Successors()
		if t.Op() == sil.CondBr && len(t.Args()) > 0 {
			if v, ok := constantCondition(t.Args()[0]); ok {
				if v {
					succs = []*sil.Block{t.Aux().Dest}
				} else {
					succs = []*sil.Block{t.Aux().Else}
				}
			}
		}
		for _, s := range succs {
			if s != nil && !seen[s] {
				seen[s] = true
				work = append(work, s)
			}
		}
	}
	return false
}

// constantCondition is the value of a condition that is a literal: an
// integer_literal, or the one a Bool literal wraps and a branch unwraps.
func constantCondition(v *sil.Value) (bool, bool) {
	for v != nil && v.Inst() != nil {
		in := v.Inst()
		switch in.Op() {
		case sil.IntegerLiteral:
			return in.Aux().Int != 0, true
		case sil.StructExtract, sil.Struct:
			if len(in.Args()) != 1 {
				return false, false
			}
			v = in.Args()[0]
		default:
			return false, false
		}
	}
	return false, false
}
