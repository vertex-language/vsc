package types

// Layout follows Swift's, because a Vertex value and a Swift value of
// the same type are the same bytes.
//
// Three numbers describe a type, and Swift keeps them apart:
//
//	size    the bytes a value occupies
//	stride  the bytes between consecutive elements of an array —
//	        the size rounded up to the alignment, and never zero
//	align   the address multiple a value must start at
//
// A struct's size excludes its tail padding, which is why size and
// stride differ for `struct { var a: Int8; var b: Int64 }`: 9 and 16.
//
// The other rule worth naming is what an Optional costs. `Int?` is
// nine bytes — eight for the value, one to say whether it is there —
// but `String?`, `AnyObject?` and `Bool?` are the same size as what
// they wrap, because those types have representations no value uses
// and the nil case takes one. Swift calls them extra inhabitants;
// hasSpareValues is where this file decides which types have them.

// Target specifies target machine parameters for type layout calculations.
type Target struct {
	WordSize int64
	Align    int64
}

// DefaultTarget64 is a 64-bit standard target (e.g. ARM64 or x86-64).
var DefaultTarget64 = &Target{
	WordSize: 8,
	Align:    8,
}

func alignUp(offset, align int64) int64 {
	if align <= 1 {
		return offset
	}
	rem := offset % align
	if rem == 0 {
		return offset
	}
	return offset + (align - rem)
}

// Alignof returns the alignment of t in bytes on target.
func Alignof(t Type, target *Target) int64 {
	if target == nil {
		target = DefaultTarget64
	}
	if t == nil {
		return 1
	}

	switch tt := t.Underlying().(type) {
	case *Basic:
		switch tt.kind {
		case Bool, Int8, UInt8:
			return 1
		case Int16, UInt16:
			return 2
		case Int32, UInt32, Float:
			return 4
		case Int64, UInt64, Double:
			return 8
		case Void, Never:
			return 1
		default:
			// Int, UInt, and the two-word String and Character.
			return target.WordSize
		}
	case *Class, *Array, *Dictionary, *Signature, *Metatype,
		*Existential, *Protocol:
		return target.WordSize
	case *Optional:
		return Alignof(tt.Wrapped, target)
	// Two bounds, laid out as a struct of two of them would be.
	case *Range:
		return Alignof(tt.Element, target)
	case *Tuple:
		var maxAlign int64 = 1
		for _, elem := range tt.Elements {
			if a := Alignof(elem.Type, target); a > maxAlign {
				maxAlign = a
			}
		}
		return maxAlign
	case *Struct:
		var maxAlign int64 = 1
		for _, f := range tt.Fields {
			if a := Alignof(f.Type, target); a > maxAlign {
				maxAlign = a
			}
		}
		return maxAlign
	case *Enum:
		var maxAlign int64 = 1
		for _, c := range tt.Cases {
			if a := Alignof(c.AssociatedType, target); a > maxAlign {
				maxAlign = a
			}
		}
		return maxAlign
	default:
		return 1
	}
}

// Sizeof returns the size of t in bytes on target: the bytes a value
// occupies, tail padding excluded.
func Sizeof(t Type, target *Target) int64 {
	if target == nil {
		target = DefaultTarget64
	}
	if t == nil {
		return 0
	}

	switch tt := t.Underlying().(type) {
	case *Basic:
		switch tt.kind {
		case Void, Never:
			return 0
		case Bool, Int8, UInt8:
			return 1
		case Int16, UInt16:
			return 2
		case Int32, UInt32, Float:
			return 4
		case Int64, UInt64, Double:
			return 8
		case Int, UInt:
			return target.WordSize
		case String, Character:
			return target.WordSize * 2
		default:
			return target.WordSize
		}

	// A reference, a buffer pointer, or a metatype: one word.
	case *Class, *Array, *Dictionary, *Metatype:
		return target.WordSize

	// A function value is a pair: the code, and the context it
	// captured.
	case *Signature:
		return target.WordSize * 2

	// An existential is a three-word inline buffer, the value's
	// metadata, and one witness table per protocol. `Any` witnesses
	// nothing and is four words.
	case *Existential:
		return target.WordSize * int64(4+len(tt.Protocols))
	case *Protocol:
		return target.WordSize * 5

	// Two bounds. A ClosedRange is the same two: Swift's carries a
	// finished bit in its *iterator*, not in the range.
	case *Range:
		return Strideof(tt.Element, target) + Sizeof(tt.Element, target)

	case *Optional:
		size := Sizeof(tt.Wrapped, target)
		if !hasSpareValues(tt.Wrapped) {
			size++
		}
		return size

	case *Tuple:
		var offset int64
		for _, elem := range tt.Elements {
			offset = alignUp(offset, Alignof(elem.Type, target))
			offset += Sizeof(elem.Type, target)
		}
		return offset

	case *Struct:
		var offset int64
		for _, f := range tt.Fields {
			offset = alignUp(offset, Alignof(f.Type, target))
			offset += Sizeof(f.Type, target)
		}
		return offset

	// An enum with no associated values is its tag alone, and one
	// with a single case does not need even that. One with payloads
	// is the largest of them plus the tag.
	case *Enum:
		var maxPayload int64
		for _, c := range tt.Cases {
			if s := Sizeof(c.AssociatedType, target); s > maxPayload {
				maxPayload = s
			}
		}
		if maxPayload == 0 {
			return tagSize(len(tt.Cases))
		}
		return maxPayload + tagSize(len(tt.Cases))

	default:
		return target.WordSize
	}
}

// tagSize is the bytes an enum spends saying which case it holds.
func tagSize(cases int) int64 {
	switch {
	case cases <= 1:
		return 0
	case cases <= 1<<8:
		return 1
	case cases <= 1<<16:
		return 2
	default:
		return 4
	}
}

// Strideof returns the stride of t: its size rounded up to its
// alignment, and at least one byte, so that an array of an empty type
// still has distinct element addresses.
func Strideof(t Type, target *Target) int64 {
	stride := alignUp(Sizeof(t, target), Alignof(t, target))
	if stride == 0 {
		return 1
	}
	return stride
}

// hasSpareValues reports whether t leaves bit patterns no value of it
// uses, which is what lets `T?` be the size of `T`. A reference has
// one because null is not an object; a Bool has 254 because it uses
// two of its byte's 256 values; an Int has none, because every
// pattern is a number.
//
// A compound type inherits the answer from what it holds: Swift lays
// the nil case in the spare values of whichever stored member has
// some.
func hasSpareValues(t Type) bool {
	if t == nil {
		return false
	}
	switch tt := t.Underlying().(type) {
	case *Basic:
		switch tt.kind {
		case Bool, String, Character:
			return true
		default:
			return false
		}
	case *Class, *Array, *Dictionary, *Signature, *Metatype,
		*Existential, *Protocol:
		return true
	case *Struct:
		for _, f := range tt.Fields {
			if hasSpareValues(f.Type) {
				return true
			}
		}
		return false
	case *Tuple:
		for _, elem := range tt.Elements {
			if hasSpareValues(elem.Type) {
				return true
			}
		}
		return false
	// An enum with no payload spends one byte on a handful of cases
	// and leaves the rest of it spare. One with a payload has already
	// spent what it had on its own tag.
	case *Enum:
		for _, c := range tt.Cases {
			if c.AssociatedType != nil {
				return false
			}
		}
		return len(tt.Cases) > 1 && len(tt.Cases) < 1<<8
	default:
		// An Optional has spent its own spare values on the tag, and
		// a type this file cannot see through is assumed to have
		// none.
		return false
	}
}

// Offsetof is where a stored property begins, in bytes from the first
// one. Fields are laid out in the order they were written, each at the
// next offset its own alignment admits: nothing is reordered, so a
// program that cares can see what it declared.
//
// For a struct or a tuple that is the offset from the start of the
// value. A class's stored properties do not start at the start of the
// object -- an instance begins with whatever the runtime needs to know
// about it -- so a caller adds the header itself, since how big that
// is belongs to the runtime rather than to the type.
//
// The second result is false when the type has no such field.
func Offsetof(t Type, field string, target *Target) (int64, bool) {
	if target == nil {
		target = DefaultTarget64
	}
	if t == nil {
		return 0, false
	}
	var fields []*Field
	switch tt := t.Underlying().(type) {
	case *Struct:
		fields = tt.Fields
	case *Class:
		fields = ClassFields(tt)
	case *Tuple:
		var offset int64
		for i, elem := range tt.Elements {
			offset = alignUp(offset, Alignof(elem.Type, target))
			if elem.Name == field || itoa(i) == field {
				return offset, true
			}
			offset += Sizeof(elem.Type, target)
		}
		return 0, false
	default:
		return 0, false
	}

	var offset int64
	for _, f := range fields {
		offset = alignUp(offset, Alignof(f.Type, target))
		if f.Name == field {
			return offset, true
		}
		offset += Sizeof(f.Type, target)
	}
	return 0, false
}

// InstanceSizeof is how much storage a class instance's stored
// properties need, laid out the way Offsetof walks them.
//
// It is not Sizeof, and the difference is the whole reason it exists.
// Sizeof of a class is one word, because a class *value* is a
// reference — which is the right answer for a register, a parameter
// and a field, and a catastrophic one for an allocation. A class with
// four Int properties would be given eight bytes and write thirty-two
// into them.
//
// The header is not included: what precedes the first property is the
// runtime's business and the runtime adds it.
//
// It reports false for a type that is not a class, and for one whose
// properties have no layout.
func InstanceSizeof(t Type, target *Target) (int64, bool) {
	if target == nil {
		target = DefaultTarget64
	}
	if t == nil {
		return 0, false
	}
	cl, ok := t.Underlying().(*Class)
	if !ok {
		return 0, false
	}
	var size, maxAlign int64 = 0, 1
	for _, f := range ClassFields(cl) {
		if f == nil || f.Type == nil {
			return 0, false
		}
		fs := Sizeof(f.Type, target)
		fa := Alignof(f.Type, target)
		if fs < 0 || fa <= 0 {
			return 0, false
		}
		if fa > maxAlign {
			maxAlign = fa
		}
		size = alignUp(size, fa) + fs
	}
	// Rounded up, so that an array of them would be aligned and so
	// that two instances allocated back to back cannot overlap.
	return alignUp(size, maxAlign), true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ClassFields is every stored property an instance of cl has, the
// superclass's first.
//
// A class declares its own properties and inherits the rest, and the
// inherited ones come first because a subclass has to be usable
// wherever its base is: the base's fields must sit at the offsets the
// base's own code reads them at. Class.Fields holds what the
// declaration wrote and nothing else, so a subclass laid out from it
// alone had no room for what it inherited -- every field started at
// zero and a write to one clobbered another.
func ClassFields(cl *Class) []*Field {
	if cl == nil {
		return nil
	}
	var chain []*Class
	seen := map[*Class]bool{}
	for c := cl; c != nil && !seen[c]; {
		seen[c] = true
		chain = append([]*Class{c}, chain...)
		next, _ := c.Superclass.(*Class)
		if next == nil && c.Superclass != nil {
			next, _ = c.Superclass.Underlying().(*Class)
		}
		c = next
	}
	var out []*Field
	for _, c := range chain {
		out = append(out, c.Fields...)
	}
	return out
}
