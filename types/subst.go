package types

// Substitute replaces occurrences of type parameters in t with types from the subst map.
// If no substitution occurs, the original type t is returned.
func Substitute(t Type, subst map[*TypeParam]Type) Type {
	return substitute(t, subst, nil)
}

// substitute is Substitute with the types already being rewritten, so
// that one reachable from itself terminates.
//
// A class whose property is another class that names it back, or a
// struct with a method returning its own type, is a cycle in the type
// graph -- and walking one without remembering where it started is a
// walk that does not stop. What is returned on the second visit is
// the type as it was: it is already being rewritten further up, and
// the copy that walk makes is the one every reference will see.
func substitute(t Type, subst map[*TypeParam]Type, seen map[Type]bool) Type {
	if t == nil || len(subst) == 0 {
		return t
	}
	switch t.(type) {
	case *Struct, *Class, *Enum:
		if seen[t] {
			return t
		}
		if seen == nil {
			seen = make(map[Type]bool, 4)
		}
		seen[t] = true
		defer delete(seen, t)
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
		elem := substitute(tt.Elem, subst, seen)
		if elem == tt.Elem {
			return tt
		}
		return &Array{Elem: elem}

	case *Dictionary:
		k := substitute(tt.Key, subst, seen)
		v := substitute(tt.Value, subst, seen)
		if k == tt.Key && v == tt.Value {
			return tt
		}
		return &Dictionary{Key: k, Value: v}

	case *Optional:
		wrapped := substitute(tt.Wrapped, subst, seen)
		if wrapped == tt.Wrapped {
			return tt
		}
		return &Optional{Wrapped: wrapped}

	case *Metatype:
		inst := substitute(tt.Instance, subst, seen)
		if inst == tt.Instance {
			return tt
		}
		return &Metatype{Instance: inst}

	case *Tuple:
		changed := false
		elems := make([]*TupleElement, len(tt.Elements))
		for i, el := range tt.Elements {
			newTyp := substitute(el.Type, subst, seen)
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
			newTyp := substitute(p.Type, subst, seen)
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
		res := substitute(tt.Results, subst, seen)
		if res != tt.Results {
			changed = true
		}
		var thrown Type
		if tt.Thrown != nil {
			thrown = substitute(tt.Thrown, subst, seen)
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

	// A nominal type's own parts. Substituting one is what makes
	// `Box<Int32>` a struct with an Int32 in it rather than a struct
	// with a T in it -- which is what every question about layout,
	// triviality and registers is really asking.
	case *Struct:
		fields, fchanged := substFields(tt.Fields, subst, seen)
		methods, mchanged := substMethods(tt.Methods, subst, seen)
		if !fchanged && !mchanged {
			return t
		}
		out := *tt
		out.Fields, out.Methods = fields, methods
		out.TypeParams = nil
		return &out
	case *Class:
		fields, fchanged := substFields(tt.Fields, subst, seen)
		methods, mchanged := substMethods(tt.Methods, subst, seen)
		if !fchanged && !mchanged {
			return t
		}
		out := *tt
		out.Fields, out.Methods = fields, methods
		out.TypeParams = nil
		return &out
	case *Enum:
		changed := false
		cases := make([]*EnumCase, len(tt.Cases))
		for i, c := range tt.Cases {
			if c == nil {
				continue
			}
			sub := substitute(c.AssociatedType, subst, seen)
			if sub != c.AssociatedType {
				changed = true
			}
			copy := *c
			copy.AssociatedType = sub
			cases[i] = &copy
		}
		methods, mchanged := substMethods(tt.Methods, subst, seen)
		if !changed && !mchanged {
			return t
		}
		out := *tt
		out.Cases, out.Methods = cases, methods
		out.TypeParams = nil
		return &out

	case *GenericInstance:
		changed := false
		args := make([]Type, len(tt.Args))
		for i, a := range tt.Args {
			newA := substitute(a, subst, seen)
			if newA != a {
				changed = true
			}
			args[i] = newA
		}
		base := substitute(tt.Base, subst, seen)
		if base != tt.Base {
			changed = true
		}
		if !changed {
			return tt
		}
		return &GenericInstance{Base: base, Args: args}

	// A dependent member type: `C.Element` once C is known.
	//
	// Substituting the base is not the answer, it is what makes the
	// answer askable: `C.Element` with C standing for Ints is Ints's
	// choice of Element, which the conformer recorded when it
	// conformed. Where the base is still a parameter -- one generic
	// calling another, both still generic -- the dependency stands and
	// is substituted again further down.
	case *Dependent:
		base := substitute(tt.Base, subst, seen)
		if base == tt.Base {
			return tt
		}
		return DependentOn(base, tt.Name)

	case *Named:
		if tt.underlying != nil {
			newU := substitute(tt.underlying, subst, seen)
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

// substFields is a nominal type's stored properties with a
// substitution applied.
func substFields(in []*Field, subst map[*TypeParam]Type, seen map[Type]bool) ([]*Field, bool) {
	changed := false
	out := make([]*Field, len(in))
	for i, f := range in {
		if f == nil {
			continue
		}
		sub := substitute(f.Type, subst, seen)
		if sub != f.Type {
			changed = true
		}
		copy := *f
		copy.Type = sub
		out[i] = &copy
	}
	return out, changed
}

// substMethods is a nominal type's methods with a substitution
// applied to their signatures.
func substMethods(in []*Method, subst map[*TypeParam]Type, seen map[Type]bool) ([]*Method, bool) {
	changed := false
	out := make([]*Method, len(in))
	for i, m := range in {
		if m == nil {
			continue
		}
		sub, _ := substitute(m.Sig, subst, seen).(*Signature)
		if sub == nil {
			sub = m.Sig
		}
		if sub != m.Sig {
			changed = true
		}
		copy := *m
		copy.Sig = sub
		out[i] = &copy
	}
	return out, changed
}

// AssocOf is what a type chose for an associated type of this name,
// or nil where it made no such choice.
//
// A conformer's choices are on the nominal type, so this is where a
// dependent member type stops being a dependency: `C.Element` with C
// standing for Ints is what Ints said Element is.
func AssocOf(t Type, name string) Type {
	if t == nil {
		return nil
	}
	// An instance of a generic type answers with its own arguments
	// substituted -- `Box<Int>.Element` is what Box said, with T
	// standing for Int.
	if inst, ok := t.(*GenericInstance); ok {
		if answer := AssocOf(inst.Base, name); answer != nil {
			params := typeParamsOf(inst.Base)
			sub := make(map[*TypeParam]Type, len(params))
			for i, p := range params {
				if i < len(inst.Args) {
					sub[p] = inst.Args[i]
				}
			}
			return Substitute(answer, sub)
		}
		return nil
	}
	switch n := t.Underlying().(type) {
	case *Struct:
		return n.Assoc[name]
	case *Class:
		return n.Assoc[name]
	case *Enum:
		return n.Assoc[name]
	}
	return nil
}

// typeParamsOf is a nominal type's type parameters.
func typeParamsOf(t Type) []*TypeParam {
	switch n := t.Underlying().(type) {
	case *Struct:
		return n.TypeParams
	case *Class:
		return n.TypeParams
	case *Enum:
		return n.TypeParams
	}
	return nil
}

// DependentOn is what `base.name` denotes: the answer where one is
// known, and the dependency where none is.
//
// Three things can be known. A `where` clause may have said what it
// is outright. A concrete type will have chosen when it conformed.
// And where neither has, the answer still depends on the parameter,
// which is what a Dependent is for.
func DependentOn(base Type, name string) Type {
	if tp, ok := base.(*TypeParam); ok {
		if bound := tp.Bound[name]; bound != nil {
			return bound
		}
		return &Dependent{Base: base, Name: name}
	}
	if answer := AssocOf(base, name); answer != nil {
		return answer
	}
	return &Dependent{Base: base, Name: name}
}
