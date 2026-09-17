package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/derive"
	"github.com/vertex-language/vsc/types"
)

// An operator is a function like any other, and Swift finds one in two
// places: declared at the top level of a module -- this one, one it
// imports, or the standard library -- and declared `static` on the type
// of one of its operands. `static func + (a: Duration, b: Duration)` is
// the usual way to write one, and what makes `a + b` mean it.

// isOperatorName reports whether a function's name is an operator's.
func isOperatorName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '&', '@', '/', '=', '>', '<', '*', '!', '|', '+', '?', '%', '-', '~', '^', '.':
		default:
			return false
		}
	}
	return true
}

// unlabelOperator takes the labels off an operator's parameters. They are
// never written at a use -- `a + b` has nowhere to write one -- so Swift
// gives an operator none, whatever its declaration names them, and its
// symbol says so.
func unlabelOperator(name string, sig *types.Signature) {
	if sig == nil || !isOperatorName(name) {
		return
	}
	for _, p := range sig.Params {
		p.Label = "_"
	}
}

// operatorChoice is one declaration an operator could mean: a function,
// or a static method of a type.
type operatorChoice struct {
	fn  *FuncSymbol
	ref *MethodRef
}

func (o operatorChoice) sig() *types.Signature {
	if o.fn != nil {
		return o.fn.Signature()
	}
	return o.ref.Method.Sig
}

// operatorChoices is every declaration of op that takes as many operands:
// the static ones on the operands' own types first, then this module's,
// the imported modules', and core's.
func (c *checker) operatorChoices(scope *Scope, op string, operands []types.Type) []operatorChoice {
	var out []operatorChoice
	seenType := map[types.Type]bool{}
	for _, t := range operands {
		if t == nil || isInvalid(t) {
			continue
		}
		recv, methods := methodsNamed(&types.Metatype{Instance: t}, op)
		if len(methods) == 0 {
			if brecv, bm := c.builtinMethods(&types.Metatype{Instance: t}, op); len(bm) > 0 {
				recv, methods = brecv, bm
			}
		}
		if seenType[recv] {
			continue
		}
		// A struct's derived `==` is found as one; see package derive.
		if op == "==" && len(methods) == 0 {
			_, methods = methodsNamed(&types.Metatype{Instance: t}, derive.EqualsName)
		}
		if op == "==" && len(methods) == 0 {
			_, methods = methodsNamed(&types.Metatype{Instance: t}, derive.EnumEqualsName)
		}
		seenType[recv] = true
		for _, m := range methods {
			if m.Sig != nil && len(m.Sig.Params) == len(operands) {
				out = append(out, operatorChoice{ref: &MethodRef{Recv: recv, Method: m}})
			}
		}
		out = append(out, requirementOperators(t, op, len(operands))...)
	}

	seen := map[*FuncSymbol]bool{}
	add := func(s *Scope) {
		if s == nil {
			return
		}
		fs, ok := s.elems[op].(*FuncSymbol)
		if !ok {
			return
		}
		for _, f := range fs.Overloads() {
			if seen[f] {
				continue
			}
			seen[f] = true
			if sig := f.Signature(); sig != nil && len(sig.Params) == len(operands) {
				out = append(out, operatorChoice{fn: f})
			}
		}
	}
	// A function's own scopes, out to the module's: an operator is only
	// ever declared at the top, but that is also where a type scope's
	// static methods would be found by name, so type scopes are passed.
	for s := scope; s != nil; s = s.parent {
		if !s.members {
			add(s)
		}
	}
	for _, m := range c.modules {
		add(m)
	}
	return out
}

// requirementOperators is what a generic parameter's constraints promise
// of op: `==` on a T that is Equatable is Equatable's requirement, with
// Self standing for T. Which type's operator it is, is answered when the
// function is specialized.
func requirementOperators(t types.Type, op string, arity int) []operatorChoice {
	var cons []types.Type
	switch tt := t.(type) {
	case *types.TypeParam:
		cons = tt.Constraints
	case *types.Dependent:
		cons = associatedConstraints(tt)
	default:
		return nil
	}
	var out []operatorChoice
	for _, con := range cons {
		p, ok := con.Underlying().(*types.Protocol)
		if !ok {
			continue
		}
		for _, up := range allProtocols([]*types.Protocol{p}) {
			for _, r := range up.Requirements {
				if r == nil || r.Name != op || r.Sig == nil || len(r.Sig.Params) != arity {
					continue
				}
				sig, _ := throughParam(up, t, r.Sig).(*types.Signature)
				if sig == nil {
					continue
				}
				m := &types.Method{Name: r.Name, Sig: sig, IsStatic: true}
				out = append(out, operatorChoice{ref: &MethodRef{Recv: up, Method: m}})
			}
		}
	}
	return out
}

// resolveOperator picks the declaration an operator applied to operands
// means, records it for lowering, and returns its result. Operands that
// fit exactly are preferred to ones that fit by being assignable, and
// both to literals retyped to fit.
func (c *checker) resolveOperator(scope *Scope, op string, e ast.Expr, xs []ast.Expr, operands []types.Type) (types.Type, bool) {
	if ch, ok := c.pickOperator(scope, op, xs, operands); ok {
		return c.chooseOperator(e, ch), true
	}
	if t, ok := c.derivedOperator(scope, op, e, xs, operands); ok {
		return t, true
	}
	return nil, false
}

// pickOperator is resolveOperator's choice, without recording it.
func (c *checker) pickOperator(scope *Scope, op string, xs []ast.Expr, operands []types.Type) (operatorChoice, bool) {
	choices := c.operatorChoices(scope, op, operands)
	if len(choices) == 0 {
		return operatorChoice{}, false
	}
	fits := func(ch operatorChoice, exact bool) bool {
		for i, t := range operands {
			want := ch.sig().Params[i].Type
			if exact && !types.Identical(t, want) {
				return false
			}
			if !types.AssignableTo(t, want) {
				return false
			}
		}
		return true
	}
	for _, exact := range []bool{true, false} {
		for _, ch := range choices {
			if fits(ch, exact) {
				return ch, true
			}
		}
	}
	// Literals retyped to what a declaration takes: `d * 4` with d a
	// Duration and `*` taking an Int64. Only beside another operand: a
	// lone literal is already the type its context wants, and `-128` where
	// an Int8 goes is not an Int negated.
	if len(operands) < 2 {
		return operatorChoice{}, false
	}
	for _, ch := range choices {
		ok := true
		for i, t := range operands {
			want := ch.sig().Params[i].Type
			if types.AssignableTo(t, want) {
				continue
			}
			if i >= len(xs) {
				ok = false
				break
			}
			if nt, adopted := c.adoptTree(xs[i], want, scope); !adopted || !types.AssignableTo(nt, want) {
				ok = false
				break
			}
		}
		if ok {
			return ch, true
		}
	}
	return operatorChoice{}, false
}

// A DerivedOperator is an operator a protocol gives a type that declares
// another: Equatable's `!=` is `==` negated, and Comparable's `>`, `<=`
// and `>=` are `<` with its operands swapped, negated, or both.
type DerivedOperator struct {
	// Fn or Method is the operator it is written in terms of.
	Fn     *FuncSymbol
	Method *MethodRef
	// Swap passes the operands the other way round; Negate inverts the
	// answer.
	Swap, Negate bool
}

// derivedOperator resolves `!=`, `>`, `<=` or `>=` on two values of a type
// that is Equatable or Comparable and declares only `==` or `<`.
func (c *checker) derivedOperator(scope *Scope, op string, e ast.Expr, xs []ast.Expr, operands []types.Type) (types.Type, bool) {
	if len(operands) != 2 || operands[0] == nil || !types.Identical(operands[0], operands[1]) {
		return nil, false
	}
	var base, proto string
	var d DerivedOperator
	switch op {
	case "!=":
		base, proto, d.Negate = "==", "Equatable", true
	case ">":
		base, proto, d.Swap = "<", "Comparable", true
	case "<=":
		base, proto, d.Swap, d.Negate = "<", "Comparable", true, true
	case ">=":
		base, proto, d.Negate = "<", "Comparable", true
	default:
		return nil, false
	}
	p, ok := c.coreProtocol(proto)
	if !ok || !c.conformsTo(operands[0].Underlying(), p) {
		return nil, false
	}
	ch, ok := c.pickOperator(scope, base, xs, operands)
	if !ok {
		return nil, false
	}
	d.Fn, d.Method = ch.fn, ch.ref
	c.info.DerivedOperators[e] = &d
	return types.Typ[types.Bool], true
}

// coreProtocol is the protocol core declares under name.
func (c *checker) coreProtocol(name string) (*types.Protocol, bool) {
	core := c.modules["Swift"]
	if core == nil {
		return nil, false
	}
	tn, ok := core.elems[name].(*TypeNameSymbol)
	if !ok {
		return nil, false
	}
	p, ok := tn.Type().(*types.Protocol)
	return p, ok
}

// chooseOperator records a choice and returns its result type.
func (c *checker) chooseOperator(e ast.Expr, ch operatorChoice) types.Type {
	if ch.ref != nil {
		c.info.OperatorMethods[e] = ch.ref
	} else {
		c.info.Operators[e] = ch.fn
	}
	return ch.sig().Results
}

// implicitOperandContext is the type a leading-dot operand is read as,
// given the other operand's type: the parameter type of the first static
// operator on that type the leading-dot member can be found in. With
// `t - .Seconds(1)`, `-` on Timestamp takes a Duration and a Timestamp,
// and `.Seconds` is only a Duration's. Where nothing answers, the other
// operand's own type is the context, as it is for `k == .escape`.
func (c *checker) implicitOperandContext(scope *Scope, op string, other types.Type, implicit ast.Expr, implicitIndex int) types.Type {
	if other == nil || isInvalid(other) {
		return other
	}
	operands := []types.Type{other, other}
	for _, ch := range c.operatorChoices(scope, op, operands) {
		if ch.ref == nil {
			continue
		}
		params := ch.sig().Params
		if !types.AssignableTo(other, params[1-implicitIndex].Type) {
			continue
		}
		want := params[implicitIndex].Type
		if c.fitsQuietly(implicit, want, scope) {
			return want
		}
	}
	return other
}

// fitsQuietly reports whether e checks as want without a diagnostic, and
// leaves no trace of having tried.
func (c *checker) fitsQuietly(e ast.Expr, want types.Type, scope *Scope) bool {
	quiet := len(c.info.Diagnostics)
	t := c.checkExpr(e, want, scope)
	ok := len(c.info.Diagnostics) == quiet && t != nil && !isInvalid(t) && types.AssignableTo(t, want)
	c.info.Diagnostics = c.info.Diagnostics[:quiet]
	return ok
}
