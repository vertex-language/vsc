package types

// Typ is an array of built-in basic types indexed by BasicKind.
var Typ = [...]*Basic{
	Invalid:       {kind: Invalid, info: 0, name: "invalid"},
	Bool:          {kind: Bool, info: IsBoolean, name: "Bool"},
	Int:           {kind: Int, info: IsInteger, name: "Int"},
	Int8:          {kind: Int8, info: IsInteger, name: "Int8"},
	Int16:         {kind: Int16, info: IsInteger, name: "Int16"},
	Int32:         {kind: Int32, info: IsInteger, name: "Int32"},
	Int64:         {kind: Int64, info: IsInteger, name: "Int64"},
	UInt:          {kind: UInt, info: IsInteger | IsUnsigned, name: "UInt"},
	UInt8:         {kind: UInt8, info: IsInteger | IsUnsigned, name: "UInt8"},
	UInt16:        {kind: UInt16, info: IsInteger | IsUnsigned, name: "UInt16"},
	UInt32:        {kind: UInt32, info: IsInteger | IsUnsigned, name: "UInt32"},
	UInt64:        {kind: UInt64, info: IsInteger | IsUnsigned, name: "UInt64"},
	Float:         {kind: Float, info: IsFloat, name: "Float"},
	Double:        {kind: Double, info: IsFloat, name: "Double"},
	String:        {kind: String, info: IsString, name: "String"},
	Character:     {kind: Character, info: 0, name: "Character"},
	Void:          {kind: Void, info: 0, name: "Void"},
	Never:         {kind: Never, info: 0, name: "Never"},
	UntypedBool:   {kind: UntypedBool, info: IsBoolean | IsUntyped, name: "untyped bool"},
	UntypedInt:    {kind: UntypedInt, info: IsInteger | IsUntyped, name: "untyped int"},
	UntypedFloat:  {kind: UntypedFloat, info: IsFloat | IsUntyped, name: "untyped float"},
	UntypedString: {kind: UntypedString, info: IsString | IsUntyped, name: "untyped string"},
	UntypedNil:    {kind: UntypedNil, info: IsUntyped, name: "untyped nil"},
}

// The universe is in two tiers, and the order matters.
//
// The first is Swift's, and is the language's own set of names: Int,
// UInt8, Double, String, Character, Bool, Void, Never, Any. A program
// written in Swift sees these and nothing else, and they are what a
// diagnostic names a type by.
//
// The second is Vertex's, and is optional. int32 beside Int32, string
// beside String: aliases the language offers in addition, never in
// place of the names above. Removing one would change what Vertex
// accepts; removing one of the first tier would change what Swift
// accepts, which is not a thing this compiler may do.
var swiftTypes = map[string]Type{
	"Bool":      Typ[Bool],
	"Int":       Typ[Int],
	"Int8":      Typ[Int8],
	"Int16":     Typ[Int16],
	"Int32":     Typ[Int32],
	"Int64":     Typ[Int64],
	"UInt":      Typ[UInt],
	"UInt8":     Typ[UInt8],
	"UInt16":    Typ[UInt16],
	"UInt32":    Typ[UInt32],
	"UInt64":    Typ[UInt64],
	"Float":     Typ[Float],
	"Double":    Typ[Double],
	"String":    Typ[String],
	"Character": Typ[Character],
	"Void":      Typ[Void],
	"Never":     Typ[Never],
	"Any":       &Existential{},

	// The protocols the compiler knows itself, rather than reading
	// from a library: what `throws` throws, what crosses an isolation
	// boundary, and the two a type suppresses with `~`.
	"Error":     ErrorProtocol,
	"Sendable":  SendableProtocol,
	"Copyable":  CopyableProtocol,
	"Escapable": EscapableProtocol,
	"AnyObject": AnyObjectProtocol,
}

// The compiler-known protocols. They are not declared in any source
// this compiler reads, and the language would not work without them:
// a throw needs an Error, and `~Copyable` needs a Copyable to
// suppress.
var (
	ErrorProtocol     = &Protocol{Name: "Error"}
	SendableProtocol  = &Protocol{Name: "Sendable"}
	CopyableProtocol  = &Protocol{Name: "Copyable"}
	EscapableProtocol = &Protocol{Name: "Escapable"}
	AnyObjectProtocol = &Protocol{Name: "AnyObject"}
)

// vertexTypes are the additional lowercase spellings. Each is an
// alias for one of the names above and denotes the same type.
var vertexTypes = map[string]Type{
	"bool":    Typ[Bool],
	"int":     Typ[Int],
	"int8":    Typ[Int8],
	"int16":   Typ[Int16],
	"int32":   Typ[Int32],
	"int64":   Typ[Int64],
	"uint":    Typ[UInt],
	"uint8":   Typ[UInt8],
	"uint16":  Typ[UInt16],
	"uint32":  Typ[UInt32],
	"uint64":  Typ[UInt64],
	"float":   Typ[Float],
	"float32": Typ[Float],
	"double":  Typ[Double],
	"float64": Typ[Double],
	"string":  Typ[String],
	"char":    Typ[Character],
	"void":    Typ[Void],
	"never":   Typ[Never],
	"any":     &Existential{},
}

// LookupUniverse returns the built-in type corresponding to name, or
// nil if there is none. Swift's names are looked up first: a program
// that writes only those is reading the same universe swiftc does.
func LookupUniverse(name string) Type {
	if t, ok := swiftTypes[name]; ok {
		return t
	}
	return vertexTypes[name]
}

// LookupSwiftUniverse returns the built-in type for name, considering
// only the names Swift itself has. It is what a Swift-only mode reads
// the universe through.
func LookupSwiftUniverse(name string) Type {
	return swiftTypes[name]
}
