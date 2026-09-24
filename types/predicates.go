package types

// Identical reports whether x and y are identical types.
// SameDecl reports whether two function signatures belong to the same
// declaration, which is a different question from whether they are the
// same type.
//
// Swift's full name for a function is its base name and its argument
// labels together, so `label(a:)` and `label(b:)` are two declarations
// and may both exist. Their *types* are identical -- SE-0111 took
// labels out of the type system -- which is why Identical says yes to
// the same pair and why the two questions cannot share an answer:
// asking Identical about a redeclaration rejected every overload that
// differed only in its labels, and asking SameDecl about an assignment
// would reject every declared function used as a value.
func SameDecl(x, y *Signature) bool {
	if !Identical(x, y) {
		return false
	}
	if x == nil || y == nil {
		return x == y
	}
	for i, p := range x.Params {
		if p.Label != y.Params[i].Label {
			return false
		}
	}
	return true
}

func Identical(x, y Type) bool {
	if x == y {
		return true
	}
	if x == nil || y == nil {
		return false
	}
	// A parameter an extension's where clause fixes is that type.
	if tp, ok := x.(*TypeParam); ok && tp.Same != nil {
		return Identical(tp.Same, y)
	}
	if tp, ok := y.(*TypeParam); ok && tp.Same != nil {
		return Identical(x, tp.Same)
	}

	// A typealias is another spelling of the type it names, not a new
	// one, so it is looked through. A Named with nothing under it is
	// a name this compiler could not resolve, and stays opaque.
	if n, ok := x.(*Named); ok && n.underlying != nil {
		return Identical(n.Underlying(), y)
	}
	if n, ok := y.(*Named); ok && n.underlying != nil {
		return Identical(x, n.Underlying())
	}

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
	case *Set:
		if yt, ok := y.(*Set); ok {
			return Identical(xt.Elem, yt.Elem)
		}
	case *Optional:
		if yt, ok := y.(*Optional); ok {
			return Identical(xt.Wrapped, yt.Wrapped)
		}
	// Two pointers are one type when they point at the same thing in
	// the same way. Mutability is part of it: `UnsafePointer<T>` and
	// `UnsafeMutablePointer<T>` are the difference between `const T *`
	// and `T *`, and Swift keeps them apart too.
	case *Pointer:
		if yt, ok := y.(*Pointer); ok {
			return xt.Mutable == yt.Mutable && xt.Opaque == yt.Opaque &&
				(xt.Elem == nil) == (yt.Elem == nil) &&
				(xt.Elem == nil || Identical(xt.Elem, yt.Elem))
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
			if xt.Throws != yt.Throws {
				return false
			}
			if (xt.Thrown == nil) != (yt.Thrown == nil) {
				return false
			}
			if xt.Thrown != nil && !Identical(xt.Thrown, yt.Thrown) {
				return false
			}
			if !Identical(xt.Results, yt.Results) {
				return false
			}
			// An argument label is part of a declaration's name and
			// not of its type: SE-0111 took labels out of the type
			// system, so `func triple(_ n: Int32) -> Int32` has type
			// `(Int32) -> Int32` and is assignable to a variable of
			// it. swiftc accepts both spellings against the same
			// annotation, and comparing labels here rejected every
			// declared function used as a value.
			for i, p := range xt.Params {
				yp := yt.Params[i]
				if p.Ownership != yp.Ownership || p.Variadic != yp.Variadic || !Identical(p.Type, yp.Type) {
					return false
				}
			}
			return true
		}
	case *TypeParam:
		if yt, ok := y.(*TypeParam); ok {
			return xt.Name == yt.Name
		}
	// Two dependent member types are the same type when they are
	// reached through the same parameter and name the same associated
	// type. `C.Item` is `C.Item`, however many times it was written.
	case *Dependent:
		if yt, ok := y.(*Dependent); ok {
			return xt.Name == yt.Name && Identical(xt.Base, yt.Base)
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

// ConformsTo reports whether t conforms to proto.
//
// Conformance is nominal, as it is in Swift: a type conforms to a
// protocol because it was declared to, in its own declaration or in
// an extension, and never because its members happen to line up. Two
// types with the same shape are not interchangeable, which is the
// whole point of a protocol being a name.
func ConformsTo(t Type, proto *Protocol) bool {
	if t == nil || proto == nil {
		return false
	}
	if proto.Name == "Any" {
		return true
	}
	// Every class is AnyObject, and so is an existential of one.
	if proto == AnyObjectProtocol {
		switch u := t.Underlying().(type) {
		case *Class:
			return true
		case *Existential:
			for _, p := range u.Protocols {
				if ConformsTo(p, proto) {
					return true
				}
			}
			return false
		}
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

	// The standard library's own types conform to its protocols: every
	// number, Bool, String and Character is Equatable and Hashable and
	// describes itself, and all but Bool are Comparable.
	case *Basic:
		switch proto.Name {
		case "Equatable", "Hashable", "CustomStringConvertible":
			return tt.info&(IsNumeric|IsBoolean|IsString) != 0 || tt.kind == Character
		case "Comparable":
			return tt.info&(IsNumeric|IsString) != 0 || tt.kind == Character
		// The numbers' protocols; see core.swift.
		case "AdditiveArithmetic", "Numeric", "ExpressibleByIntegerLiteral":
			return tt.info&IsNumeric != 0 && tt.info&IsUntyped == 0
		case "SignedNumeric":
			return tt.info&IsNumeric != 0 && tt.info&IsUnsigned == 0 && tt.info&IsUntyped == 0
		case "BinaryInteger", "FixedWidthInteger":
			return tt.info&IsInteger != 0 && tt.info&IsUntyped == 0
		case "SignedInteger":
			return tt.info&IsInteger != 0 && tt.info&IsUnsigned == 0 && tt.info&IsUntyped == 0
		case "UnsignedInteger":
			return tt.info&IsInteger != 0 && tt.info&IsUnsigned != 0 && tt.info&IsUntyped == 0
		case "FloatingPoint", "BinaryFloatingPoint", "ExpressibleByFloatLiteral":
			return tt.info&IsFloat != 0 && tt.info&IsUntyped == 0
		}
		return false

	// An Optional, an Array, a Set and a Dictionary are Equatable and
	// Hashable where what they hold is.
	case *Optional:
		switch proto.Name {
		case "Equatable", "Hashable":
			return ConformsTo(tt.Wrapped, proto)
		}
		return false
	case *Array:
		switch proto.Name {
		case "Equatable", "Hashable":
			return ConformsTo(tt.Elem, proto)
		}
		return false

	case *Struct:
		return containsProto(tt.Conformances)

	case *Class:
		if containsProto(tt.Conformances) {
			return true
		}
		return tt.Superclass != nil && ConformsTo(tt.Superclass, proto)

	case *Enum:
		if containsProto(tt.Conformances) {
			return true
		}
		// An enum whose cases carry nothing is Equatable and Hashable
		// without saying so.
		if proto.Name == "Equatable" || proto.Name == "Hashable" {
			for _, c := range tt.Cases {
				if c != nil && c.AssociatedType != nil {
					return false
				}
			}
			return len(tt.Cases) > 0
		}
		return false

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

// AssignableTo reports whether a value of type from is assignable to a variable of type to.
func AssignableTo(from, to Type) bool {
	if from == nil || to == nil {
		return false
	}
	if Identical(from, to) {
		return true
	}
	if tp, ok := from.(*TypeParam); ok && tp.Same != nil {
		return AssignableTo(tp.Same, to)
	}
	if tp, ok := to.(*TypeParam); ok && tp.Same != nil {
		return AssignableTo(from, tp.Same)
	}
	// Never is the bottom type, assignable to anything
	if from == Typ[Never] {
		return true
	}
	// A tuple is one of the same elements with labels added or taken
	// away: (x: Int, y: Int) and (Int, Int) are each other's, as in
	// Swift. Labels both written must agree.
	if ft, ok := from.(*Tuple); ok {
		if tt, ok := to.(*Tuple); ok && len(ft.Elements) == len(tt.Elements) && len(ft.Elements) > 1 {
			for i, el := range ft.Elements {
				other := tt.Elements[i]
				if el.Name != "" && other.Name != "" && el.Name != other.Name {
					return false
				}
				if !Identical(el.Type, other.Type) {
					return false
				}
			}
			return true
		}
	}

	// Inside a generic type's declaration its own name alone is the
	// instance of its own parameters: `self` in TaskGroup<R> is a
	// TaskGroup<R>.
	if selfInstance(from, to) || selfInstance(to, from) {
		return true
	}

	// A function that cannot fail goes where one that may is wanted,
	// as in Swift: the error edge is never taken.
	if fs, ok := from.(*Signature); ok {
		if ts, ok := to.(*Signature); ok && ts.Throws && !fs.Throws && !fs.Async && !ts.Async &&
			len(fs.TypeParams) == 0 && len(ts.TypeParams) == 0 {
			relaxed := *fs
			relaxed.Throws, relaxed.Rethrows, relaxed.Thrown = ts.Throws, ts.Rethrows, ts.Thrown
			relaxed.Isolated = ts.Isolated
			return Identical(&relaxed, ts)
		}
	}

	// A metatype is assignable to the existential metatype of what its
	// instance is assignable to: Double.Type to Any.Type, and a
	// conforming type's to P.Type.
	if fm, ok := from.(*Metatype); ok {
		if tm, ok := to.(*Metatype); ok {
			if _, isEx := tm.Instance.(*Existential); isEx {
				return AssignableTo(fm.Instance, tm.Instance)
			}
		}
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
		// Through the name, not at it: a typealias is another
		// spelling of the type it names, so a literal fits `Num`
		// exactly when it fits the Int32 that Num is. Asserting on
		// the type as written meant a parameter declared with an
		// alias took no literal at all.
		if bTo, ok := to.Underlying().(*Basic); ok {
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
			if _, isOpt := to.(*Optional); isOpt {
				return true
			}
		}
		// A type of a program's own, or the core's Character, takes the
		// literals it says it is expressible by.
		if _, isBasic := to.Underlying().(*Basic); !isBasic {
			for _, p := range LiteralProtocols(bFrom.kind) {
				if ConformsToNamed(to, p) {
					return true
				}
			}
		}
		if bFrom.kind == UntypedNil {
			return false
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
	// Two addresses compare as two numbers, whatever is at either
	// end. `p == q` asks whether they are the same place.
	case *Pointer:
		return true
	case *Tuple:
		for _, elem := range tt.Elements {
			if !Comparable(elem.Type) {
				return false
			}
		}
		return true
	// Arrays are equal element by element.
	case *Array:
		return Comparable(tt.Elem)
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

// ConformsToNamed reports whether t conforms to the standard library's
// protocol of that name -- by the rules ConformsTo has for the core's own
// types, and for any other by the conformances it declares, directly or
// through a protocol that inherits the one named.
func ConformsToNamed(t Type, name string) bool {
	if t == nil {
		return false
	}
	var declared []*Protocol
	switch tt := t.(type) {
	case *Basic, *Optional, *Array:
		return ConformsTo(t, &Protocol{Name: name})
	case *GenericInstance:
		return ConformsToNamed(tt.Base, name)
	case *Named:
		if tt.underlying != nil {
			return ConformsToNamed(tt.underlying, name)
		}
		return false
	case *Struct:
		declared = tt.Conformances
	case *Enum:
		if ConformsTo(tt, &Protocol{Name: name}) {
			return true
		}
		declared = tt.Conformances
	case *TypeParam:
		for _, c := range tt.Constraints {
			if p, ok := c.Underlying().(*Protocol); ok {
				declared = append(declared, p)
			}
		}
	case *Class:
		declared = tt.Conformances
		if tt.Superclass != nil && ConformsToNamed(tt.Superclass, name) {
			return true
		}
	}
	var walk func(ps []*Protocol) bool
	walk = func(ps []*Protocol) bool {
		for _, p := range ps {
			if p != nil && (p.Name == name || walk(p.Inherited)) {
				return true
			}
		}
		return false
	}
	return walk(declared)
}

// LiteralProtocols are the protocols by which a type may be written as an
// untyped literal of this kind, the most particular first: a string
// literal is an ExpressibleByStringLiteral's, or a Character's through
// ExpressibleByExtendedGraphemeClusterLiteral.
func LiteralProtocols(kind BasicKind) []string {
	switch kind {
	case UntypedInt:
		return []string{"ExpressibleByIntegerLiteral"}
	case UntypedFloat:
		return []string{"ExpressibleByFloatLiteral"}
	case UntypedBool:
		return []string{"ExpressibleByBooleanLiteral"}
	case UntypedString:
		return []string{"ExpressibleByStringLiteral", "ExpressibleByExtendedGraphemeClusterLiteral",
			"ExpressibleByUnicodeScalarLiteral"}
	case UntypedNil:
		return []string{"ExpressibleByNilLiteral"}
	}
	return nil
}

// selfInstance reports whether bare is a generic nominal type and inst
// the instance of it whose arguments are its own parameters.
func selfInstance(bare, inst Type) bool {
	gi, ok := inst.(*GenericInstance)
	if !ok || gi.Base != bare {
		return false
	}
	params := typeParamsOf(bare)
	if len(params) == 0 || len(params) != len(gi.Args) {
		return false
	}
	for i, p := range params {
		if tp, ok := gi.Args[i].(*TypeParam); !ok || tp != p {
			return false
		}
	}
	return true
}
