package types

// Identical reports whether x and y are identical types.
func Identical(x, y Type) bool {
	if x == y {
		return true
	}
	if x == nil || y == nil {
		return false
	}

	// Unwrap Named wrappers if both are not named nominal types, or check underlying
	switch xt := x.(type) {
	case *Basic:
		if yt, ok := y.(*Basic); ok {
			return xt.kind == yt.kind
		}
	case *Named:
		if yt, ok := y.(*Named); ok {
			return xt.Name == yt.Name && xt.Pkg == yt.Pkg
		}
	case *Struct:
		if yt, ok := y.(*Struct); ok {
			return xt == yt || (xt.Name != "" && xt.Name == yt.Name)
		}
	case *Class:
		if yt, ok := y.(*Class); ok {
			return xt == yt || (xt.Name != "" && xt.Name == yt.Name)
		}
	case *Enum:
		if yt, ok := y.(*Enum); ok {
			return xt == yt || (xt.Name != "" && xt.Name == yt.Name)
		}
	case *Protocol:
		if yt, ok := y.(*Protocol); ok {
			return xt == yt || (xt.Name != "" && xt.Name == yt.Name)
		}
	case *Array:
		if yt, ok := y.(*Array); ok {
			return Identical(xt.Elem, yt.Elem)
		}
	case *Dictionary:
		if yt, ok := y.(*Dictionary); ok {
			return Identical(xt.Key, yt.Key) && Identical(xt.Value, yt.Value)
		}
	case *Optional:
		if yt, ok := y.(*Optional); ok {
			return Identical(xt.Wrapped, yt.Wrapped)
		}
	case *Metatype:
		if yt, ok := y.(*Metatype); ok {
			return Identical(xt.Instance, yt.Instance)
		}
	case *Tuple:
		if yt, ok := y.(*Tuple); ok {
			if len(xt.Elements) != len(yt.Elements) {
				return false
			}
			for i, elem := range xt.Elements {
				if elem.Name != yt.Elements[i].Name || !Identical(elem.Type, yt.Elements[i].Type) {
					return false
				}
			}
			return true
		}
	case *Signature:
		if yt, ok := y.(*Signature); ok {
			if len(xt.Params) != len(yt.Params) || xt.Async != yt.Async {
				return false
			}
			if (xt.Throws == nil) != (yt.Throws == nil) {
				return false
			}
			if xt.Throws != nil && !Identical(xt.Throws, yt.Throws) {
				return false
			}
			if !Identical(xt.Results, yt.Results) {
				return false
			}
			for i, p := range xt.Params {
				yp := yt.Params[i]
				if p.Label != yp.Label || p.Ownership != yp.Ownership || p.Variadic != yp.Variadic || !Identical(p.Type, yp.Type) {
					return false
				}
			}
			return true
		}
	case *TypeParam:
		if yt, ok := y.(*TypeParam); ok {
			return xt.Name == yt.Name
		}
	case *GenericInstance:
		if yt, ok := y.(*GenericInstance); ok {
			if !Identical(xt.Base, yt.Base) || len(xt.Args) != len(yt.Args) {
				return false
			}
			for i, arg := range xt.Args {
				if !Identical(arg, yt.Args[i]) {
					return false
				}
			}
			return true
		}
	}
	return false
}

// ConformsTo reports whether type t conforms to protocol proto.
func ConformsTo(t Type, proto *Protocol) bool {
	if t == nil || proto == nil {
		return false
	}
	if proto.Name == "Any" {
		return true
	}

	containsProto := func(list []*Protocol) bool {
		for _, p := range list {
			if Identical(p, proto) || ConformsTo(p, proto) {
				return true
			}
		}
		return false
	}

	switch tt := t.(type) {
	case *Protocol:
		if Identical(tt, proto) {
			return true
		}
		return containsProto(tt.Inherited)

	case *Struct:
		if containsProto(tt.Conformances) {
			return true
		}
		return satisfiesRequirements(tt.Fields, tt.Methods, proto)

	case *Class:
		if containsProto(tt.Conformances) {
			return true
		}
		if tt.Superclass != nil && ConformsTo(tt.Superclass, proto) {
			return true
		}
		return satisfiesRequirements(tt.Fields, tt.Methods, proto)

	case *Enum:
		if containsProto(tt.Conformances) {
			return true
		}
		return satisfiesRequirements(nil, tt.Methods, proto)

	case *GenericInstance:
		return ConformsTo(tt.Base, proto)

	case *Existential:
		for _, p := range tt.Protocols {
			if ConformsTo(p, proto) {
				return true
			}
		}
		return false

	case *TypeParam:
		for _, c := range tt.Constraints {
			if ConformsTo(c, proto) {
				return true
			}
		}
		return false

	case *Named:
		if tt.underlying != nil {
			return ConformsTo(tt.underlying, proto)
		}
		return false

	default:
		return false
	}
}

func satisfiesRequirements(fields []*Field, methods []*Method, proto *Protocol) bool {
	if len(proto.Requirements) == 0 {
		return false
	}
	for _, req := range proto.Requirements {
		satisfied := false
		if req.Sig != nil {
			for _, m := range methods {
				if m.Name == req.Name && Identical(m.Sig, req.Sig) {
					satisfied = true
					break
				}
			}
		} else if req.Type != nil {
			for _, f := range fields {
				if f.Name == req.Name && AssignableTo(f.Type, req.Type) {
					satisfied = true
					break
				}
			}
		}
		if !satisfied {
			return false
		}
	}
	return true
}

// AssignableTo reports whether a value of type from is assignable to a variable of type to.
func AssignableTo(from, to Type) bool {
	if from == nil || to == nil {
		return false
	}
	if Identical(from, to) {
		return true
	}
	// Never is the bottom type, assignable to anything
	if from == Typ[Never] {
		return true
	}

	// Any / Existential conformance
	if ex, ok := to.(*Existential); ok {
		if len(ex.Protocols) == 0 {
			return true // Any accepts everything
		}
		for _, p := range ex.Protocols {
			if !ConformsTo(from, p) {
				return false
			}
		}
		return true
	}

	// Protocol target
	if proto, ok := to.(*Protocol); ok {
		return ConformsTo(from, proto)
	}

	// Untyped literals
	if bFrom, ok := from.(*Basic); ok && bFrom.info&IsUntyped != 0 {
		if bTo, ok := to.(*Basic); ok {
			switch bFrom.kind {
			case UntypedInt:
				return bTo.info&IsNumeric != 0
			case UntypedFloat:
				return bTo.info&IsFloat != 0
			case UntypedBool:
				return bTo.info&IsBoolean != 0
			case UntypedString:
				return bTo.info&IsString != 0
			}
		}
		if bFrom.kind == UntypedNil {
			_, isOpt := to.(*Optional)
			return isOpt
		}
	}

	// Optional promotion: T is assignable to T?
	if toOpt, ok := to.(*Optional); ok {
		if AssignableTo(from, toOpt.Wrapped) {
			return true
		}
	}

	// GenericInstance compatibility
	if genFrom, ok := from.(*GenericInstance); ok {
		if genTo, ok := to.(*GenericInstance); ok {
			if Identical(genFrom.Base, genTo.Base) && len(genFrom.Args) == len(genTo.Args) {
				match := true
				for i, a := range genFrom.Args {
					if !AssignableTo(a, genTo.Args[i]) {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
	}

	// Class inheritance: subclass is assignable to superclass
	if fromClass, ok := from.(*Class); ok {
		if toClass, ok := to.(*Class); ok {
			curr := fromClass.Superclass
			for curr != nil {
				if Identical(curr, toClass) {
					return true
				}
				if c, ok := curr.(*Class); ok {
					curr = c.Superclass
				} else {
					break
				}
			}
		}
	}

	return false
}

// Comparable reports whether t supports equality operations.
func Comparable(t Type) bool {
	if t == nil {
		return false
	}
	switch tt := t.(type) {
	case *Basic:
		return tt.info&(IsNumeric|IsBoolean|IsString) != 0 || tt.kind == Character
	case *Optional:
		return Comparable(tt.Wrapped)
	case *Tuple:
		for _, elem := range tt.Elements {
			if !Comparable(elem.Type) {
				return false
			}
		}
		return true
	case *Enum:
		return true
	case *Class:
		return true // reference identity
	default:
		return false
	}
}

// IsCopyable reports whether values of type t can be implicitly copied.
func IsCopyable(t Type) bool {
	if t == nil {
		return true
	}
	switch tt := t.Underlying().(type) {
	case *Struct:
		return tt.Copyable
	case *Enum:
		return tt.Copyable
	default:
		return true
	}
}
