package types

// Type layout calculations for size, stride, and alignment.

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
		case Int16, UInt16, Float16, BFloat16:
			return 2
		case Int32, UInt32, Float:
			return 4
		case Int64, UInt64, Double:
			return 8
		case Void, Never:
			return 1
		default:
			return target.WordSize
		}
	case *Class, *Array, *Dictionary, *Set, *Signature, *Metatype,
		*Existential, *Protocol:
		return target.WordSize
	case *Pointer:
		return target.WordSize
	case *Optional:
		return Alignof(tt.Wrapped, target)
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
		// An enum with a payload is lowered as whole words -- its
		// payload and tag packed into as many as they take, see
		// lower's enumImage -- so inside a struct it starts on a
		// word, whatever its payload's own alignment. What the
		// lowering stores and what the metadata says must agree, or
		// an array of the struct copies from where nothing was
		// written.
		var maxAlign int64 = 1
		for _, c := range tt.Cases {
			if c.AssociatedType != nil {
				return target.WordSize
			}
		}
		return maxAlign
	default:
		return 1
	}
}

// Sizeof returns the size of t in bytes on target.
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
		case Int16, UInt16, Float16, BFloat16:
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

	case *Class, *Array, *Dictionary, *Set, *Metatype:
		return target.WordSize
	case *Pointer:
		return target.WordSize
	case *Signature:
		return target.WordSize * 2
	case *Existential:
		return target.WordSize * int64(4+len(tt.Protocols))
	case *Protocol:
		return target.WordSize * 5
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

	case *Enum:
		var maxPayload int64
		for _, c := range tt.Cases {
			var s int64
			if c.Indirect && c.AssociatedType != nil {
				s = target.WordSize
			} else {
				s = Sizeof(c.AssociatedType, target)
			}
			if s > maxPayload {
				maxPayload = s
			}
		}
		if maxPayload == 0 {
			return tagSize(len(tt.Cases))
		}
		// A payload enum is lowered as whole words, each stored whole,
		// so it takes whole words: nothing may sit in the part of its
		// last word the payload and tag leave, or a store would
		// overwrite it. See lower's enumImage.
		return alignUp(maxPayload+tagSize(len(tt.Cases)), target.WordSize)

	default:
		return target.WordSize
	}
}

// tagSize returns the number of bytes required to store the enum case tag.
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

// Strideof returns the stride of t in bytes (size rounded up to alignment).
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
	case *Class, *Array, *Dictionary, *Set, *Signature, *Metatype,
		*Existential, *Protocol:
		return true
	case *Pointer:
		return true
	// A struct or a tuple spares a representation where one of its
	// words is never zero -- a reference, a String's object, a function's
	// code -- which nil is all zeros beside. A Bool or an enum inside one
	// travels in as few bits as it needs and has no value to spare there,
	// so an optional of it has a tag byte (see lower's neverZeroWord).
	case *Struct:
		for _, f := range tt.Fields {
			if neverZero(f.Type) {
				return true
			}
		}
		return false
	case *Tuple:
		for _, elem := range tt.Elements {
			if neverZero(elem.Type) {
				return true
			}
		}
		return false
	case *Enum:
		for _, c := range tt.Cases {
			if c.AssociatedType != nil {
				return false
			}
		}
		return len(tt.Cases) > 1 && len(tt.Cases) < 1<<8
	default:
		return false
	}
}

// Offsetof returns the byte offset of a stored property within type t.
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

// InstanceSizeof returns the storage size required for a class instance's stored fields.
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

// ClassFields returns all stored properties of cl, with inherited superclass fields first.
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

// neverZero reports whether a value of t has a word that is never zero,
// as lower's neverZeroWord finds one.
func neverZero(t Type) bool {
	switch tt := t.Underlying().(type) {
	case *Class, *Array, *Dictionary, *Set, *Signature:
		return true
	case *Basic:
		return tt.kind == String
	case *Struct:
		for _, f := range tt.Fields {
			if neverZero(f.Type) {
				return true
			}
		}
	}
	return false
}
