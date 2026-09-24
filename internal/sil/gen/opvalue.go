package gen

import (
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/internal/sil"
	"github.com/vertex-language/vsc/mangle"
	"github.com/vertex-language/vsc/types"
)

// An operator named as a value -- `reduce(0, +)` -- is a function value.
// One the program declares is that function; one core declares is an
// instruction, which has no address, so a function that applies it is
// written once per operator and type and handed over instead, as swiftc
// writes a thunk for the same reason.

// operatorValue lowers an operator written as a value.
func (g *gen) operatorValue(e *ast.OperatorExpr) *sil.Value {
	if ref := g.info.OperatorMethods[e]; ref != nil {
		// A requirement's operator -- `+` of a T: Zeroable -- is, in a
		// specialization, the one the concrete type declares.
		// The concrete type is what the operands are, substituted.
		if _, abstract := ref.Recv.(*types.Protocol); abstract && ref.Method.Sig != nil && len(ref.Method.Sig.Params) > 0 {
			if resolved, ok := g.witness(ref, g.substituted(ref.Method.Sig.Params[0].Type)); ok {
				ref = resolved
			}
		} else if tp, ok := ref.Recv.(*types.TypeParam); ok {
			if resolved, ok := g.witness(ref, g.substituted(tp)); ok {
				ref = resolved
			}
		}
		symbol := g.staticSymbol(ref, ref.Recv)
		callee := g.m.Func(symbol).SetSourceName(ref.Method.Name)
		if g.needsType(callee) {
			g.declareSignature(callee, ref.Method.Sig)
		}
		v := g.blk.ThinToThickFunction(g.blk.FunctionRef(callee), lowerType(ref.Method.Sig))
		g.destroyLater(v)
		return v
	}
	sym, _ := g.info.Operators[e].(*analyzer.FuncSymbol)
	if sym == nil {
		g.refuse(e, "an operator used as a value that the checker did not resolve")
		return nil
	}
	if spec, generic := g.info.OperatorSpecs[e]; generic {
		return g.specializedValue(e, sym, spec)
	}
	if !g.coreOperator(sym) {
		return g.funcValue(sym)
	}
	sig := sym.Signature()
	if len(sig.Params) != 2 {
		g.refuse(e, "a prefix operator used as a value")
		return nil
	}
	thunk := g.operatorThunk(e, sym, sig)
	if thunk == nil {
		return nil
	}
	v := g.blk.ThinToThickFunction(g.blk.FunctionRef(thunk), lowerType(sig))
	g.destroyLater(v)
	return v
}

// operatorThunk is the function that applies a core operator to its two
// parameters, written once per operator and operand type.
func (g *gen) operatorThunk(e *ast.OperatorExpr, sym *analyzer.FuncSymbol, sig *types.Signature) *sil.Func {
	name, err := mangle.Function(mangle.Decl{
		Module:    g.module,
		Name:      sym.Name(),
		Signature: sig,
		ModuleOf:  g.moduleOfType,
	})
	if err != nil {
		g.refuse(e, "an operator used as a value this compiler cannot name: "+err.Error())
		return nil
	}
	name += "Tf"
	if existing := g.m.Lookup(name); existing != nil && !existing.IsDeclaration() {
		return existing
	}
	f := g.m.Func(name).SetSourceName(sym.Name()).SetLinkage(sil.Hidden).SetAttr("ossa")

	outer := struct {
		fn     *sil.Func
		entry  bool
		blk    *sil.Block
		scopes []*scope
		locals map[analyzer.Symbol]*local
		loops  []loop
		recv   types.Type
		self   *local
	}{g.fn, g.entry, g.blk, g.scopes, g.locals, g.loops, g.recv, g.self}
	defer func() {
		g.fn, g.entry, g.blk = outer.fn, outer.entry, outer.blk
		g.scopes, g.locals, g.loops = outer.scopes, outer.locals, outer.loops
		g.recv, g.self = outer.recv, outer.self
	}()
	g.fn, g.entry, g.recv, g.self = f, false, nil, nil
	g.locals = map[analyzer.Symbol]*local{}
	g.scopes, g.loops = nil, nil
	f.Type().Params = nil
	g.push()
	g.blk = f.Entry()

	var args []*sil.Value
	for _, p := range sig.Params {
		t := lowerType(p.Type)
		args = append(args, f.Param(t, paramConvention(p, t)))
	}
	f.SetResult(lowerType(sig.Results), resultConvention(lowerType(sig.Results)))
	v := g.operate(e, sym.Name(), sig.Params[0].Type, sig.Results, args[0], args[1])
	if v == nil {
		if bit, ok := g.compareBit(sym.Name(), sig.Params[0].Type, args[0], args[1]); ok {
			v = g.boolOf(bit)
		}
	}
	if v == nil {
		g.refuse(e, "an operator used as a value that this compiler cannot apply")
		g.pop()
		return nil
	}
	g.unwind()
	g.blk.Return(g.consume(v))
	g.pop()
	return f
}

// compareBit is a core comparison of two values, where the operator is
// one, as the bit it answers.
func (g *gen) compareBit(op string, t types.Type, a, b *sil.Value) (*sil.Value, bool) {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
	default:
		return nil, false
	}
	bit := g.compare(op, a, b, t)
	return bit, bit != nil
}
