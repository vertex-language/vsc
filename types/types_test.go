package types

import (
	"testing"
)

func TestBasicTypes(t *testing.T) {
	if Typ[Int].String() != "Int" {
		t.Errorf("expected Int, got %s", Typ[Int])
	}
	if Typ[String].String() != "String" {
		t.Errorf("expected String, got %s", Typ[String])
	}
	if Typ[Bool].Info()&IsBoolean == 0 {
		t.Errorf("expected Bool to have IsBoolean info")
	}
	if Typ[Int32].Info()&IsInteger == 0 {
		t.Errorf("expected Int32 to have IsInteger info")
	}
	if Typ[Float].Info()&IsFloat == 0 {
		t.Errorf("expected Float to have IsFloat info")
	}

	// LookupUniverse
	if LookupUniverse("Int") != Typ[Int] {
		t.Errorf("LookupUniverse(Int) failed")
	}
	if LookupUniverse("int32") != Typ[Int32] {
		t.Errorf("LookupUniverse(int32) failed")
	}
	if LookupUniverse("NonExistent") != nil {
		t.Errorf("expected nil for non-existent type")
	}
}

func TestCompositeTypes(t *testing.T) {
	// Array [Int]
	arr := &Array{Elem: Typ[Int]}
	if arr.String() != "[Int]" {
		t.Errorf("expected [Int], got %s", arr)
	}

	// Dictionary [String: Int]
	dict := &Dictionary{Key: Typ[String], Value: Typ[Int]}
	if dict.String() != "[String: Int]" {
		t.Errorf("expected [String: Int], got %s", dict)
	}

	// Optional Double?
	opt := &Optional{Wrapped: Typ[Double]}
	if opt.String() != "Double?" {
		t.Errorf("expected Double?, got %s", opt)
	}

	// Metatype
	meta := &Metatype{Instance: Typ[Int]}
	if meta.String() != "Int.Type" {
		t.Errorf("expected Int.Type, got %s", meta)
	}

	// Tuple (x: Int, y: Double)
	tup := &Tuple{
		Elements: []*TupleElement{
			{Name: "x", Type: Typ[Int]},
			{Name: "y", Type: Typ[Double]},
		},
	}
	if tup.String() != "(x: Int, y: Double)" {
		t.Errorf("expected '(x: Int, y: Double)', got %s", tup)
	}

	// Signature (borrowing a: Int, _ b: String...) async throws -> Bool
	sig := &Signature{
		Params: []*Param{
			{Label: "a", Name: "x", Type: Typ[Int], Ownership: Borrowing},
			{Label: "_", Name: "y", Type: Typ[String], Variadic: true},
		},
		Results: Typ[Bool],
		Async:   true,
		Throws:  Typ[Never],
	}
	expectedSig := "(borrowing a x: Int, _ y: String...) async throws -> Bool"
	if sig.String() != expectedSig {
		t.Errorf("expected %q, got %q", expectedSig, sig.String())
	}

	// Named nominal type
	alias := NewNamed("MyInt", "main", Typ[Int])
	if alias.Underlying() != Typ[Int] {
		t.Errorf("expected underlying Int, got %s", alias.Underlying())
	}
	if alias.String() != "MyInt" {
		t.Errorf("expected MyInt, got %s", alias.String())
	}
}

func TestTypePredicates(t *testing.T) {
	// Identical
	if !Identical(Typ[Int], Typ[Int]) {
		t.Errorf("expected Int to be identical to Int")
	}
	if Identical(Typ[Int], Typ[String]) {
		t.Errorf("Int should not be identical to String")
	}
	arr1 := &Array{Elem: Typ[Int]}
	arr2 := &Array{Elem: Typ[Int]}
	if !Identical(arr1, arr2) {
		t.Errorf("expected arr1 identical to arr2")
	}

	// AssignableTo: Never
	if !AssignableTo(Typ[Never], Typ[Int]) {
		t.Errorf("Never should be assignable to any type")
	}

	// AssignableTo: Optional promotion
	optInt := &Optional{Wrapped: Typ[Int]}
	if !AssignableTo(Typ[Int], optInt) {
		t.Errorf("Int should be assignable to Int?")
	}

	// AssignableTo: UntypedInt
	if !AssignableTo(Typ[UntypedInt], Typ[Int32]) {
		t.Errorf("untyped int should be assignable to Int32")
	}
	if !AssignableTo(Typ[UntypedInt], Typ[Double]) {
		t.Errorf("untyped int should be assignable to Double")
	}
	if AssignableTo(Typ[UntypedInt], Typ[String]) {
		t.Errorf("untyped int should not be assignable to String")
	}

	// AssignableTo: UntypedNil to Optional
	if !AssignableTo(Typ[UntypedNil], optInt) {
		t.Errorf("nil should be assignable to Int?")
	}
	if AssignableTo(Typ[UntypedNil], Typ[Int]) {
		t.Errorf("nil should not be assignable to non-optional Int")
	}

	// Class Subtyping
	base := &Class{Name: "Base"}
	derived := &Class{Name: "Derived", Superclass: base}
	if !AssignableTo(derived, base) {
		t.Errorf("Derived should be assignable to Base")
	}
	if AssignableTo(base, derived) {
		t.Errorf("Base should not be assignable to Derived")
	}

	// Comparable
	if !Comparable(Typ[Int]) {
		t.Errorf("Int should be comparable")
	}
	if !Comparable(Typ[String]) {
		t.Errorf("String should be comparable")
	}
	if !Comparable(base) {
		t.Errorf("Class should be comparable (by reference)")
	}
	if Comparable(&Array{Elem: Typ[Int]}) {
		t.Errorf("Raw array type without protocol should not be comparable")
	}
}

func TestLayoutCalculations(t *testing.T) {
	target := DefaultTarget64

	// Primitives
	if Sizeof(Typ[Int8], target) != 1 || Alignof(Typ[Int8], target) != 1 {
		t.Errorf("Int8 layout mismatch")
	}
	if Sizeof(Typ[Int32], target) != 4 || Alignof(Typ[Int32], target) != 4 {
		t.Errorf("Int32 layout mismatch")
	}
	if Sizeof(Typ[Int64], target) != 8 || Alignof(Typ[Int64], target) != 8 {
		t.Errorf("Int64 layout mismatch")
	}
	if Sizeof(Typ[String], target) != 16 {
		t.Errorf("String size mismatch: expected 16, got %d", Sizeof(Typ[String], target))
	}

	// Struct layout with padding
	// struct { a: Int8; b: Int32 } -> a: 1 byte, pad: 3 bytes, b: 4 bytes -> total 8 bytes, align 4
	st := &Struct{
		Name: "S",
		Fields: []*Field{
			{Name: "a", Type: Typ[Int8]},
			{Name: "b", Type: Typ[Int32]},
		},
	}
	if Sizeof(st, target) != 8 {
		t.Errorf("expected struct size 8, got %d", Sizeof(st, target))
	}
	if Alignof(st, target) != 4 {
		t.Errorf("expected struct align 4, got %d", Alignof(st, target))
	}
	if Strideof(st, target) != 8 {
		t.Errorf("expected struct stride 8, got %d", Strideof(st, target))
	}

	// Struct with 64-bit alignment: struct { a: Int8; b: Int64 } -> 1 + pad 7 + 8 = 16
	st64 := &Struct{
		Name: "S64",
		Fields: []*Field{
			{Name: "a", Type: Typ[Int8]},
			{Name: "b", Type: Typ[Int64]},
		},
	}
	if Sizeof(st64, target) != 16 {
		t.Errorf("expected struct size 16, got %d", Sizeof(st64, target))
	}
	if Alignof(st64, target) != 8 {
		t.Errorf("expected struct align 8, got %d", Alignof(st64, target))
	}

	// Tuple layout: (Int8, Int32) -> 1 + pad 3 + 4 = 8
	tup := &Tuple{
		Elements: []*TupleElement{
			{Type: Typ[Int8]},
			{Type: Typ[Int32]},
		},
	}
	if Sizeof(tup, target) != 8 {
		t.Errorf("expected tuple size 8, got %d", Sizeof(tup, target))
	}
	if Alignof(tup, target) != 4 {
		t.Errorf("expected tuple align 4, got %d", Alignof(tup, target))
	}

	// Optional layout: Int32? is the value plus a byte that says
	// whether it is there — size 5, stride 8.
	opt32 := &Optional{Wrapped: Typ[Int32]}
	if Sizeof(opt32, target) != 5 {
		t.Errorf("expected Int32? size 5, got %d", Sizeof(opt32, target))
	}
	if Strideof(opt32, target) != 8 {
		t.Errorf("expected Int32? stride 8, got %d", Strideof(opt32, target))
	}
	if Alignof(opt32, target) != 4 {
		t.Errorf("expected Int32? align 4, got %d", Alignof(opt32, target))
	}

	// Primitives Void and Never
	if Sizeof(Typ[Void], target) != 0 || Sizeof(Typ[Never], target) != 0 {
		t.Errorf("expected Void/Never size 0")
	}
}

// TestLayoutMatchesSwift holds the layout rules to the numbers Swift
// reports for the same types. Every row here was read back from
// MemoryLayout on a 64-bit target, not derived.
func TestLayoutMatchesSwift(t *testing.T) {
	target := DefaultTarget64
	cls := &Class{Name: "C"}
	proto := &Protocol{Name: "P"}
	noPayload := &Enum{Name: "Three", Cases: []*EnumCase{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	onlyCase := &Enum{Name: "One", Cases: []*EnumCase{{Name: "only"}}}
	payload := &Enum{Name: "Payload", Cases: []*EnumCase{
		{Name: "a", AssociatedType: Typ[Int]},
		{Name: "b"},
	}}
	withRef := &Struct{Name: "WC", Fields: []*Field{{Name: "c", Type: cls}}}
	withInt := &Struct{Name: "WI", Fields: []*Field{{Name: "i", Type: Typ[Int]}}}
	empty := &Struct{Name: "Empty"}

	cases := []struct {
		name                  string
		typ                   Type
		size, stride, alignAt int64
	}{
		{"Bool", Typ[Bool], 1, 1, 1},
		{"Int", Typ[Int], 8, 8, 8},
		{"Float", Typ[Float], 4, 4, 4},
		{"String", Typ[String], 16, 16, 8},
		{"Character", Typ[Character], 16, 16, 8},
		{"Void", Typ[Void], 0, 1, 1},
		{"C", cls, 8, 8, 8},
		{"[Int]", &Array{Elem: Typ[Int]}, 8, 8, 8},
		{"[Int: Int]", &Dictionary{Key: Typ[Int], Value: Typ[Int]}, 8, 8, 8},
		{"() -> Void", &Signature{}, 16, 16, 8},
		{"Int.Type", &Metatype{Instance: Typ[Int]}, 8, 8, 8},
		{"Any", &Existential{}, 32, 32, 8},
		{"any P", &Existential{Protocols: []*Protocol{proto}}, 40, 40, 8},
		{"any P & Q", &Existential{Protocols: []*Protocol{proto, {Name: "Q"}}}, 48, 48, 8},
		// The Optional rule: a byte is added only where the wrapped
		// type has no spare representations of its own.
		{"Int?", &Optional{Wrapped: Typ[Int]}, 9, 16, 8},
		{"Float?", &Optional{Wrapped: Typ[Float]}, 5, 8, 4},
		{"Bool?", &Optional{Wrapped: Typ[Bool]}, 1, 1, 1},
		{"String?", &Optional{Wrapped: Typ[String]}, 16, 16, 8},
		{"C?", &Optional{Wrapped: cls}, 8, 8, 8},
		{"[Int]?", &Optional{Wrapped: &Array{Elem: Typ[Int]}}, 8, 8, 8},
		{"Int??", &Optional{Wrapped: &Optional{Wrapped: Typ[Int]}}, 10, 16, 8},
		{"WC?", &Optional{Wrapped: withRef}, 8, 8, 8},
		{"WI?", &Optional{Wrapped: withInt}, 9, 16, 8},
		{"Empty?", &Optional{Wrapped: empty}, 1, 1, 1},
		{"(C, Int)?", &Optional{Wrapped: &Tuple{Elements: []*TupleElement{
			{Type: cls}, {Type: Typ[Int]}}}}, 16, 16, 8},
		{"(Int, Int)?", &Optional{Wrapped: &Tuple{Elements: []*TupleElement{
			{Type: Typ[Int]}, {Type: Typ[Int]}}}}, 17, 24, 8},
		// Enums: the tag alone, nothing at all, or the largest
		// payload and the tag.
		{"Three", noPayload, 1, 1, 1},
		{"Three?", &Optional{Wrapped: noPayload}, 1, 1, 1},
		{"One", onlyCase, 0, 1, 1},
		{"One?", &Optional{Wrapped: onlyCase}, 1, 1, 1},
		{"Payload", payload, 9, 16, 8},
		{"Payload?", &Optional{Wrapped: payload}, 10, 16, 8},
	}
	for _, c := range cases {
		if got := Sizeof(c.typ, target); got != c.size {
			t.Errorf("Sizeof(%s) = %d, Swift says %d", c.name, got, c.size)
		}
		if got := Strideof(c.typ, target); got != c.stride {
			t.Errorf("Strideof(%s) = %d, Swift says %d", c.name, got, c.stride)
		}
		if got := Alignof(c.typ, target); got != c.alignAt {
			t.Errorf("Alignof(%s) = %d, Swift says %d", c.name, got, c.alignAt)
		}
	}
}

func TestMoreTypesAndMethods(t *testing.T) {
	// Existential & Opaque
	proto := &Protocol{Name: "CustomProto"}
	ex1 := &Existential{Protocols: []*Protocol{proto}}
	if ex1.String() != "any CustomProto" {
		t.Errorf("expected 'any CustomProto', got %q", ex1.String())
	}
	exEmpty := &Existential{}
	if exEmpty.String() != "Any" {
		t.Errorf("expected 'Any', got %q", exEmpty.String())
	}
	if !AssignableTo(Typ[Int], exEmpty) {
		t.Errorf("Int should be assignable to Any")
	}

	op1 := &Opaque{Constraints: []*Protocol{proto}}
	if op1.String() != "some CustomProto" {
		t.Errorf("expected 'some CustomProto', got %q", op1.String())
	}
	opEmpty := &Opaque{}
	if opEmpty.String() != "some Any" {
		t.Errorf("expected 'some Any', got %q", opEmpty.String())
	}

	// Enum & Cases
	enumT := &Enum{
		Name: "Color",
		Cases: []*EnumCase{
			{Name: "red", RawValue: "0"},
			{Name: "green", RawValue: "1"},
		},
	}
	if enumT.String() != "Color" || enumT.Underlying() != enumT {
		t.Errorf("enum mismatch")
	}

	// Protocol & TypeParam
	if proto.String() != "CustomProto" || proto.Underlying() != proto {
		t.Errorf("protocol mismatch")
	}
	tp := &TypeParam{Name: "T"}
	if tp.String() != "T" || tp.Underlying() != tp {
		t.Errorf("type param mismatch")
	}

	// Named SetUnderlying
	named := NewNamed("Alias", "main", nil)
	named.SetUnderlying(Typ[Int])
	if named.Underlying() != Typ[Int] {
		t.Errorf("SetUnderlying failed")
	}

	// Ownership strings
	if Borrowing.String() != "borrowing" || Consuming.String() != "consuming" || InOut.String() != "inout" || DefaultOwnership.String() != "" {
		t.Errorf("ownership string mismatch")
	}

	// Basic methods
	if Typ[Int].Kind() != Int || Typ[Int].Name() != "Int" {
		t.Errorf("basic kind/name mismatch")
	}

	// Structural Identical checks
	dict1 := &Dictionary{Key: Typ[String], Value: Typ[Int]}
	dict2 := &Dictionary{Key: Typ[String], Value: Typ[Int]}
	if !Identical(dict1, dict2) {
		t.Errorf("dict1 should be identical to dict2")
	}
	opt1 := &Optional{Wrapped: Typ[Int]}
	opt2 := &Optional{Wrapped: Typ[Int]}
	if !Identical(opt1, opt2) {
		t.Errorf("opt1 should be identical to opt2")
	}
	tup1 := &Tuple{Elements: []*TupleElement{{Name: "a", Type: Typ[Int]}}}
	tup2 := &Tuple{Elements: []*TupleElement{{Name: "a", Type: Typ[Int]}}}
	if !Identical(tup1, tup2) {
		t.Errorf("tup1 should be identical to tup2")
	}
	sig1 := &Signature{Params: []*Param{{Name: "a", Type: Typ[Int]}}, Results: Typ[Bool]}
	sig2 := &Signature{Params: []*Param{{Name: "a", Type: Typ[Int]}}, Results: Typ[Bool]}
	if !Identical(sig1, sig2) {
		t.Errorf("sig1 should be identical to sig2")
	}
	if Identical(sig1, &Signature{Async: true}) {
		t.Errorf("sig1 should not be identical to async signature")
	}
}

func TestGenericsAndSubstitution(t *testing.T) {
	tpT := &TypeParam{Name: "T"}
	tpU := &TypeParam{Name: "U"}

	// GenericInstance
	box := &Struct{Name: "Box", TypeParams: []*TypeParam{tpT}}
	boxInt := &GenericInstance{Base: box, Args: []Type{Typ[Int]}}
	boxInt2 := &GenericInstance{Base: box, Args: []Type{Typ[Int]}}
	boxString := &GenericInstance{Base: box, Args: []Type{Typ[String]}}

	if boxInt.String() != "Box<Int>" {
		t.Errorf("expected 'Box<Int>', got %q", boxInt.String())
	}
	if boxInt.Underlying() != box {
		t.Errorf("underlying of GenericInstance mismatch")
	}
	if !Identical(boxInt, boxInt2) {
		t.Errorf("boxInt should be identical to boxInt2")
	}
	if Identical(boxInt, boxString) {
		t.Errorf("boxInt should not be identical to boxString")
	}
	if !AssignableTo(boxInt, boxInt2) {
		t.Errorf("boxInt should be assignable to boxInt2")
	}
	if AssignableTo(boxInt, boxString) {
		t.Errorf("boxInt should not be assignable to boxString")
	}

	// Substitute in primitive / no-op
	if Substitute(Typ[Int], map[*TypeParam]Type{tpT: Typ[String]}) != Typ[Int] {
		t.Errorf("primitive substitute should return self")
	}

	// Substitute in TypeParam
	substMap := map[*TypeParam]Type{tpT: Typ[Int], tpU: Typ[String]}
	if !Identical(Substitute(tpT, substMap), Typ[Int]) {
		t.Errorf("substitute tpT failed")
	}
	if !Identical(Substitute(tpU, substMap), Typ[String]) {
		t.Errorf("substitute tpU failed")
	}

	// Substitute in Array [T] -> [Int]
	arrT := &Array{Elem: tpT}
	substArr := Substitute(arrT, substMap)
	if !Identical(substArr, &Array{Elem: Typ[Int]}) {
		t.Errorf("substitute array failed: got %s", substArr)
	}

	// Substitute in Dictionary [T: U] -> [Int: String]
	dictTU := &Dictionary{Key: tpT, Value: tpU}
	substDict := Substitute(dictTU, substMap)
	if !Identical(substDict, &Dictionary{Key: Typ[Int], Value: Typ[String]}) {
		t.Errorf("substitute dictionary failed: got %s", substDict)
	}

	// Substitute in Optional T? -> Int?
	optT := &Optional{Wrapped: tpT}
	substOpt := Substitute(optT, substMap)
	if !Identical(substOpt, &Optional{Wrapped: Typ[Int]}) {
		t.Errorf("substitute optional failed: got %s", substOpt)
	}

	// Substitute in Tuple (T, U) -> (Int, String)
	tupTU := &Tuple{Elements: []*TupleElement{
		{Name: "first", Type: tpT},
		{Name: "second", Type: tpU},
	}}
	substTup := Substitute(tupTU, substMap)
	if !Identical(substTup, &Tuple{Elements: []*TupleElement{
		{Name: "first", Type: Typ[Int]},
		{Name: "second", Type: Typ[String]},
	}}) {
		t.Errorf("substitute tuple failed: got %s", substTup)
	}

	// Substitute in Signature (T) -> U => (Int) -> String
	sigTU := &Signature{
		Params:  []*Param{{Name: "x", Type: tpT}},
		Results: tpU,
	}
	substSig := Substitute(sigTU, substMap)
	if !Identical(substSig, &Signature{
		Params:  []*Param{{Name: "x", Type: Typ[Int]}},
		Results: Typ[String],
	}) {
		t.Errorf("substitute signature failed: got %s", substSig)
	}

	// Substitute in GenericInstance Box<T> -> Box<Int>
	boxT := &GenericInstance{Base: box, Args: []Type{tpT}}
	substGen := Substitute(boxT, substMap)
	if !Identical(substGen, boxInt) {
		t.Errorf("substitute generic instance failed: got %s", substGen)
	}

	// SubstituteByName
	byName := SubstituteByName(arrT, map[string]Type{"T": Typ[Double]})
	if !Identical(byName, &Array{Elem: Typ[Double]}) {
		t.Errorf("SubstituteByName failed: got %s", byName)
	}
}

func TestProtocolConformance(t *testing.T) {
	protoPrintable := &Protocol{
		Name: "Printable",
		Requirements: []*Requirement{
			{Name: "description", Type: Typ[String], IsVar: true},
		},
	}
	protoEquatable := &Protocol{Name: "Equatable"}

	// Protocol inheritance
	protoCustomPrintable := &Protocol{
		Name:      "CustomPrintable",
		Inherited: []*Protocol{protoPrintable},
	}
	if !ConformsTo(protoCustomPrintable, protoPrintable) {
		t.Errorf("CustomPrintable should conform to Printable")
	}

	// Struct with direct conformance
	stPoint := &Struct{
		Name:         "Point",
		Conformances: []*Protocol{protoEquatable},
		Fields: []*Field{
			{Name: "x", Type: Typ[Int]},
			{Name: "y", Type: Typ[Int]},
			{Name: "description", Type: Typ[String]},
		},
	}
	if !ConformsTo(stPoint, protoEquatable) {
		t.Errorf("Point should conform to Equatable")
	}

	// Structural conformance via fields matching requirements
	if !ConformsTo(stPoint, protoPrintable) {
		t.Errorf("Point should structurally conform to Printable")
	}

	// Existential assignment
	anyPrintable := &Existential{Protocols: []*Protocol{protoPrintable}}
	if !AssignableTo(stPoint, anyPrintable) {
		t.Errorf("Point should be assignable to any Printable")
	}

	// Class conformance
	baseClass := &Class{
		Name:         "Animal",
		Conformances: []*Protocol{protoPrintable},
		Fields: []*Field{
			{Name: "description", Type: Typ[String]},
		},
	}
	subClass := &Class{
		Name:       "Dog",
		Superclass: baseClass,
	}
	if !ConformsTo(subClass, protoPrintable) {
		t.Errorf("Dog should conform to Printable through superclass")
	}
}

// TestUniverseTiers holds the two tiers apart. Swift's names are the
// language's own and a Swift program must find every one of them;
// Vertex's lowercase spellings are an addition, and a Swift-only
// lookup must not see them.
func TestUniverseTiers(t *testing.T) {
	swift := []string{
		"Bool", "Int", "Int8", "Int16", "Int32", "Int64",
		"UInt", "UInt8", "UInt16", "UInt32", "UInt64",
		"Float", "Double", "String", "Character", "Void", "Never", "Any",
	}
	for _, name := range swift {
		if LookupUniverse(name) == nil {
			t.Errorf("Swift's %s is not in the universe", name)
		}
		if LookupSwiftUniverse(name) == nil {
			t.Errorf("Swift's %s is not in the Swift universe", name)
		}
	}

	// Each Vertex spelling names the same type as its Swift one, and
	// is visible only through the full lookup.
	aliases := map[string]string{
		"bool": "Bool", "int": "Int", "int8": "Int8", "int16": "Int16",
		"int32": "Int32", "int64": "Int64", "uint": "UInt", "uint8": "UInt8",
		"uint16": "UInt16", "uint32": "UInt32", "uint64": "UInt64",
		"float": "Float", "float32": "Float", "double": "Double",
		"float64": "Double", "string": "String", "char": "Character",
		"void": "Void", "never": "Never",
	}
	for lower, upper := range aliases {
		got, want := LookupUniverse(lower), LookupUniverse(upper)
		if got == nil || !Identical(got, want) {
			t.Errorf("%s should name the same type as %s, got %v", lower, upper, got)
		}
		if LookupSwiftUniverse(lower) != nil {
			t.Errorf("%s is a Vertex spelling and must not be in the Swift universe", lower)
		}
	}
}
