package types

// Substitute replaces occurrences of type parameters in t with types from the subst map.
// If no substitution occurs, the original type t is returned.
func Substitute(t Type, subst map[*TypeParam]Type) Type {
	if t == nil || len(subst) == 0 {
		return t
	}

	switch tt := t.(type) {
	case *TypeParam:
		if repl, ok := subst[tt]; ok {
			return repl
		}
		// Match by name in case different pointer instances represent the same type parameter
		for param, repl := range subst {
			if param.Name == tt.Name {
				return repl
			}
		}
		return tt

	case *Array:
		elem := Substitute(tt.Elem, subst)
		if elem == tt.Elem {
			return tt
		}
		return &Array{Elem: elem}

	case *Dictionary:
		k := Substitute(tt.Key, subst)
		v := Substitute(tt.Value, subst)
		if k == tt.Key && v == tt.Value {
			return tt
		}
		return &Dictionary{Key: k, Value: v}

	case *Optional:
		wrapped := Substitute(tt.Wrapped, subst)
		if wrapped == tt.Wrapped {
			return tt
		}
		return &Optional{Wrapped: wrapped}

	case *Metatype:
		inst := Substitute(tt.Instance, subst)
		if inst == tt.Instance {
			return tt
		}
		return &Metatype{Instance: inst}

	case *Tuple:
		changed := false
		elems := make([]*TupleElement, len(tt.Elements))
		for i, el := range tt.Elements {
			newTyp := Substitute(el.Type, subst)
			if newTyp != el.Type {
				changed = true
			}
			elems[i] = &TupleElement{Name: el.Name, Type: newTyp}
		}
		if !changed {
			return tt
		}
		return &Tuple{Elements: elems}

	case *Signature:
		changed := false
		params := make([]*Param, len(tt.Params))
		for i, p := range tt.Params {
			newTyp := Substitute(p.Type, subst)
			if newTyp != p.Type {
				changed = true
			}
			params[i] = &Param{
				Name:      p.Name,
				Label:     p.Label,
				Type:      newTyp,
				Ownership: p.Ownership,
				Variadic:  p.Variadic,
			}
		}
		res := Substitute(tt.Results, subst)
		if res != tt.Results {
			changed = true
		}
		var thrown Type
		if tt.Thrown != nil {
			thrown = Substitute(tt.Thrown, subst)
			if thrown != tt.Thrown {
				changed = true
			}
		}
		if !changed {
			return tt
		}
		return &Signature{
			Params:  params,
			Results: res,
			Async:   tt.Async,
			Throws:  tt.Throws,
			Thrown:  thrown,
		}

	case *GenericInstance:
		changed := false
		args := make([]Type, len(tt.Args))
		for i, a := range tt.Args {
			newA := Substitute(a, subst)
			if newA != a {
				changed = true
			}
			args[i] = newA
		}
		base := Substitute(tt.Base, subst)
		if base != tt.Base {
			changed = true
		}
		if !changed {
			return tt
		}
		return &GenericInstance{Base: base, Args: args}

	case *Named:
		if tt.underlying != nil {
			newU := Substitute(tt.underlying, subst)
			if newU != tt.underlying {
				return NewNamed(tt.Name, tt.Pkg, newU)
			}
		}
		return tt

	default:
		return t
	}
}

// SubstituteByName replaces occurrences of type parameters matching by parameter name.
func SubstituteByName(t Type, subst map[string]Type) Type {
	if t == nil || len(subst) == 0 {
		return t
	}
	pm := make(map[*TypeParam]Type, len(subst))
	for name, typ := range subst {
		pm[&TypeParam{Name: name}] = typ
	}
	return Substitute(t, pm)
}

// Unify matches an argument's type against a parameter's, binding the
// type parameters it meets along the way, and reports whether the two
// have the same shape.
//
// This is what a generic call is inferred with: `identity(3)` unifies
// `T` with `Int`, and the result of the call is what that binding
// makes of `T`. It is a structural match and nothing more — no
// subtyping, no literal conversions, no constraint solving — which is
// what the calls this compiler can see through need.
func Unify(param, arg Type, subst map[*TypeParam]Type) bool {
	if param == nil || arg == nil {
		return false
	}
	if tp, ok := param.(*TypeParam); ok {
		if bound, seen := subst[tp]; seen {
			return Identical(bound, arg)
		}
		subst[tp] = arg
		return true
	}
	switch p := param.(type) {
	case *Optional:
		a, ok := arg.(*Optional)
		return ok && Unify(p.Wrapped, a.Wrapped, subst)
	case *Array:
		a, ok := arg.(*Array)
		return ok && Unify(p.Elem, a.Elem, subst)
	case *Dictionary:
		a, ok := arg.(*Dictionary)
		return ok && Unify(p.Key, a.Key, subst) && Unify(p.Value, a.Value, subst)
	case *Metatype:
		a, ok := arg.(*Metatype)
		return ok && Unify(p.Instance, a.Instance, subst)
	case *Tuple:
		a, ok := arg.(*Tuple)
		if !ok || len(p.Elements) != len(a.Elements) {
			return false
		}
		for i, elem := range p.Elements {
			if !Unify(elem.Type, a.Elements[i].Type, subst) {
				return false
			}
		}
		return true
	case *Signature:
		a, ok := arg.(*Signature)
		if !ok || len(p.Params) != len(a.Params) {
			return false
		}
		for i, prm := range p.Params {
			if !Unify(prm.Type, a.Params[i].Type, subst) {
				return false
			}
		}
		return Unify(p.Results, a.Results, subst)
	case *GenericInstance:
		a, ok := arg.(*GenericInstance)
		if !ok || len(p.Args) != len(a.Args) || !Identical(p.Base, a.Base) {
			return false
		}
		for i, x := range p.Args {
			if !Unify(x, a.Args[i], subst) {
				return false
			}
		}
		return true
	}
	return Identical(param, arg)
}
