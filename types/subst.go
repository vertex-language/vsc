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
		var throws Type
		if tt.Throws != nil {
			throws = Substitute(tt.Throws, subst)
			if throws != tt.Throws {
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
			Throws:  throws,
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
