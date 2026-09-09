package mangle

import (
	"errors"
	"testing"

	"github.com/vertex-language/vsc/types"
)

func sig(params []*types.Param, result types.Type) *types.Signature {
	return &types.Signature{Params: params, Results: result}
}

func p(label string, t types.Type) *types.Param {
	return &types.Param{Label: label, Type: t}
}

// TestFunction holds the mangler to strings taken from swiftc for the
// same declarations. They are written out rather than computed so that
// a change to the scheme has to be made on purpose.
func TestFunction(t *testing.T) {
	intT := types.Typ[types.Int]
	boolT := types.Typ[types.Bool]

	for _, c := range []struct {
		name string
		decl Decl
		want string
	}{
		{"no arguments, no result",
			Decl{Module: "m", Name: "f0", Signature: sig(nil, nil)},
			"$s1m2f0yyF"},

		{"Int to Int, the pair folded",
			Decl{Module: "m", Name: "f1", Signature: sig([]*types.Param{p("", intT)}, intT)},
			"$s1m2f1yS2iF"},

		{"two Ints, only the adjacent pair folds",
			Decl{Module: "m", Name: "f2", Signature: sig([]*types.Param{p("", intT), p("", intT)}, intT)},
			"$s1m2f2yS2i_SitF"},

		{"different types do not fold",
			Decl{Module: "m", Name: "f6", Signature: sig([]*types.Param{p("", intT)}, boolT)},
			"$s1m2f6ySbSiF"},

		{"a variadic parameter keeps its tuple, so the marker has a list to be in",
			Decl{Module: "m", Name: "f", Signature: sig(
				[]*types.Param{{Type: intT, Variadic: true}}, nil)},
			"$s1m1fyySid_tF"},

		{"a variadic parameter after a fixed one",
			Decl{Module: "m", Name: "g", Signature: sig(
				[]*types.Param{p("", intT), {Type: intT, Variadic: true}}, intT)},
			"$s1m1gyS2i_SidtF"},

		{"a labelled parameter keeps its tuple",
			Decl{Module: "m", Name: "f7", Signature: sig([]*types.Param{p("a", intT)}, intT)},
			"$s1m2f71aS2i_tF"},

		{"a method carries the type it is on",
			Decl{Module: "z", Context: []Nominal{{"S", Struct}}, Name: "m",
				Signature: sig([]*types.Param{p("", intT)}, intT)},
			"$s1z1SV1myS2iF"},

		{"a class method",
			Decl{Module: "z", Context: []Nominal{{"K", Class}}, Name: "m",
				Signature: sig([]*types.Param{p("", boolT)}, nil)},
			"$s1z1KC1myySbF"},

		{"an enum method",
			Decl{Module: "z", Context: []Nominal{{"E", Enum}}, Name: "m",
				Signature: sig(nil, nil)},
			"$s1z1EO1myyF"},

		{"static says so after saying function",
			Decl{Module: "z", Context: []Nominal{{"S", Struct}}, Name: "sm", Static: true,
				Signature: sig([]*types.Param{p("", intT)}, intT)},
			"$s1z1SV2smyS2iFZ"},

		{"throwing",
			Decl{Module: "n", Name: "g10", Signature: &types.Signature{Throws: true}},
			"$s1n3g10yyKF"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := Function(c.decl)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

// TestSubstitutionsFold is the compression on its own, because it is
// the part that is easy to get subtly wrong and hard to read out of a
// whole symbol.
func TestSubstitutionsFold(t *testing.T) {
	s := &types.Struct{Name: "S"}
	other := &types.Struct{Name: "T"}

	// A method whose parameter and result are both the type it is on:
	// the same index twice running, which carries a count.
	got, err := Function(Decl{
		Module: "y", Context: []Nominal{{"S", Struct}}, Name: "self1",
		Signature: sig([]*types.Param{p("", s)}, s),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "$s1y1SV5self1yA2CF"; got != want {
		t.Errorf("a repeat did not carry a count:\ngot  %s\nwant %s", got, want)
	}

	// Two different types next to each other merge into one run, and
	// every entry but the last is lowercase.
	got, err = Function(Decl{
		Module: "substitutions", Name: "h4",
		Signature: sig([]*types.Param{
			p("", &types.Struct{Name: "A"}), p("", other),
			p("", other), p("", &types.Struct{Name: "A"}),
		}, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "$s13substitutions2h4yyAA1AV_AA1TVAfDtF"; got != want {
		t.Errorf("a merged run is wrong:\ngot  %s\nwant %s", got, want)
	}
}

// TestRefusals: a symbol that is merely plausible is worse than none,
// so what this package cannot spell it declines to spell.
func TestRefusals(t *testing.T) {
	for _, c := range []struct {
		name string
		decl Decl
		err  error
	}{
		{"a constrained generic parameter",
			Decl{Module: "m", Name: "f", Signature: &types.Signature{
				TypeParams: []*types.TypeParam{
					{Name: "T", Constraints: []types.Type{&types.Protocol{Name: "P"}}},
				},
			}}, ErrUnsupported},
		{"async function",
			Decl{Module: "m", Name: "f", Signature: &types.Signature{Async: true}}, ErrUnsupported},
		{"a name that is not ASCII",
			Decl{Module: "m", Name: "café", Signature: sig(nil, nil)}, ErrName},
		{"no signature",
			Decl{Module: "m", Name: "f"}, ErrUnsupported},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := Function(c.decl)
			if err == nil {
				t.Fatalf("produced %q", got)
			}
			if !errors.Is(err, c.err) {
				t.Errorf("got %v, want %v", err, c.err)
			}
		})
	}
}

// TestDiscriminator: a private declaration carries one, so that two
// files in a module may each declare a private function of the same
// name without the two symbols colliding.
func TestDiscriminator(t *testing.T) {
	a := Discriminator("a.swift")
	b := Discriminator("b.swift")
	if a == b {
		t.Error("two files share a discriminator")
	}
	if a != Discriminator("a.swift") {
		t.Error("a file's discriminator is not stable")
	}
	if len(a) != 33 || a[0] != '_' {
		t.Errorf("the shape is %q, want an underscore and 32 more", a)
	}

	got, err := Function(Decl{
		Module: "acc", Name: "c", Discriminator: a,
		Signature: &types.Signature{},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "$s3acc1c33" + a + "LLyyF"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// TestExistentialNames: a protocol in value position is an
// existential, and Swift spells one as the protocol list followed by
// `_p`.
//
// The protocol itself carries no kind letter, where every other
// nominal does -- V for a struct, C for a class. The `_p` that closes
// the list is what says these were protocols, so writing one anyway
// produces `AA1PP_p` where swiftc writes `AA1P_p`, which is a
// different symbol.
//
// Both names below are swiftc's, read out of its own object files.
func TestExistentialNames(t *testing.T) {
	i32 := types.Typ[types.Int32]
	proto := &types.Protocol{Name: "P"}

	cases := []struct {
		name string
		decl Decl
		want string
	}{
		{"a bare protocol is an existential of itself",
			Decl{Module: "ex", Name: "call", Signature: sig(
				[]*types.Param{p("_", proto)}, i32)},
			"$s2ex4callys5Int32VAA1P_pF"},
		{"and `any P` is the same type said longer",
			Decl{Module: "ex", Name: "call", Signature: sig(
				[]*types.Param{p("_", &types.Existential{
					Protocols: []*types.Protocol{proto},
				})}, i32)},
			"$s2ex4callys5Int32VAA1P_pF"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Function(c.decl)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

// TestATypeIsSpelledWithItsOwnModule: a signature may mention a type
// from somewhere else, and the symbol has to say where.
//
// `Scale.widen(_ m: Units.Metric)` is declared in one module and
// takes a type declared in another. swiftc writes both names into the
// symbol; using the declaration's module for the parameter as well
// produces `$s5Scale5widenys5Int32VAA6MetricVF`, which is a symbol
// nothing defines and a link error with a demangled name that looks
// almost right.
//
// Both names below are swiftc's, read out of its own object files.
func TestATypeIsSpelledWithItsOwnModule(t *testing.T) {
	i32 := types.Typ[types.Int32]
	metric := &types.Struct{Name: "Metric"}
	elsewhere := func(t types.Type) string {
		if t == metric {
			return "Units"
		}
		return ""
	}

	t.Run("a parameter from another module", func(t *testing.T) {
		got, err := Function(Decl{
			Module:    "Scale",
			Name:      "widen",
			Signature: sig([]*types.Param{p("_", metric)}, i32),
			ModuleOf:  elsewhere,
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := "$s5Scale5widenys5Int32V5Units6MetricVF"; got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})

	// With nobody to ask, the declaration's own module is used, which
	// is right for a module compiled by itself.
	t.Run("with no answer, the declaration's own", func(t *testing.T) {
		got, err := Function(Decl{
			Module:    "Units",
			Name:      "widen",
			Signature: sig([]*types.Param{p("_", metric)}, i32),
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := "$s5Units5widenys5Int32VAA6MetricVF"; got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})
}

// TestANestedTypeIsSpelledAsItsChain: a type declared inside another
// is named by both, outermost first, each with its own kind letter.
//
// swiftc writes `Outer.Inner` as `AA5OuterV5InnerV` -- the module
// belongs to the outermost name and the inner one carries none. Both
// symbols below are swiftc's, read out of its own object file.
func TestANestedTypeIsSpelledAsItsChain(t *testing.T) {
	i32 := types.Typ[types.Int32]
	outer := &types.Struct{Name: "Outer"}
	inner := &types.Struct{Name: "Inner", In: outer}
	kind := &types.Enum{Name: "Kind", In: outer}

	for _, c := range []struct {
		name string
		decl Decl
		want string
	}{
		{"a struct inside a struct",
			Decl{Module: "nest", Name: "take", Signature: sig(
				[]*types.Param{p("_", inner)}, i32)},
			"$s4nest4takeys5Int32VAA5OuterV5InnerVF"},
		{"an enum inside a struct",
			Decl{Module: "nest", Name: "kindOf", Signature: sig(
				[]*types.Param{p("_", kind)}, i32)},
			"$s4nest6kindOfys5Int32VAA5OuterV4KindOF"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := Function(c.decl)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

// TestMetadataAccessorName: the symbol a client calls to get a
// class's metadata, which its allocating initializer needs as self.
//
// swiftc's own client code for `Counter(n: 1)` calls this with a
// request of zero and puts the answer in the self register. The name
// below is swiftc's, read out of the object file that client
// produced.
func TestMetadataAccessorName(t *testing.T) {
	got, err := MetadataAccessor(Decl{
		Module:  "cl",
		Context: []Nominal{{"Counter", Class}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "$s2cl7CounterCMa"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// TestGetterName: a computed property is a function that looks like a
// field, and its symbol says so.
//
// The name below is swiftc's, read out of its own object file.
func TestGetterName(t *testing.T) {
	got, err := Getter(Decl{
		Module:    "Wider",
		Context:   []Nominal{{"Vec", Struct}},
		Name:      "magnitude",
		Signature: sig(nil, types.Typ[types.Int32]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "$s5Wider3VecV9magnitudes5Int32Vvg"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// TestALoneTupleParameterKeepsItsList: a single parameter that is
// itself a tuple is written inside the parameter list, not as it.
//
// `sumTuple(_ t: (Int32, Int32))` takes one argument. Writing the
// tuple bare would spell the same symbol as a function of two, which
// is a different function -- swiftc closes the list with a second
// `_t`, and the name below is its own.
func TestALoneTupleParameterKeepsItsList(t *testing.T) {
	i32 := types.Typ[types.Int32]
	pair := &types.Tuple{Elements: []*types.TupleElement{{Type: i32}, {Type: i32}}}

	got, err := Function(Decl{
		Module:    "Wider",
		Name:      "sumTuple",
		Signature: sig([]*types.Param{p("_", pair)}, i32),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "$s5Wider8sumTupleys5Int32VAD_ADt_tF"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	// And a tuple result is written bare, which is the other half of
	// the same question.
	got, err = Function(Decl{
		Module:    "Wider",
		Name:      "pairOf",
		Signature: sig([]*types.Param{p("_", i32)}, pair),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "$s5Wider6pairOfys5Int32V_ADtADF"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// TestALabelledTupleWritesTheLabelAfterTheType: `(lo: Int32, hi:
// Int32)` is each type followed by the name it was given.
//
// Writing the label first spells a different symbol -- and one that
// demangles to the right thing, which is how it went unnoticed. The
// name below is swiftc's, read out of its own object file.
func TestALabelledTupleWritesTheLabelAfterTheType(t *testing.T) {
	i32 := types.Typ[types.Int32]
	got, err := Function(Decl{
		Module: "Tuples",
		Name:   "labelled",
		Signature: sig(nil, &types.Tuple{Elements: []*types.TupleElement{
			{Name: "lo", Type: i32},
			{Name: "hi", Type: i32},
		}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "$s6Tuples8labelleds5Int32V2lo_AD2hityF"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// TestAccessorNames: the three ways a property is reached, all of
// them swiftc's, read out of its own object file.
func TestAccessorNames(t *testing.T) {
	i32 := types.Typ[types.Int32]
	vec := Decl{Module: "Wider", Context: []Nominal{{"Vec", Struct}}}

	for _, c := range []struct {
		name string
		make func(Decl) (string, error)
		prop string
		want string
	}{
		{"an instance getter", Getter, "magnitude", "$s5Wider3VecV9magnitudes5Int32Vvg"},
		{"a static getter", StaticGetter, "unit", "$s5Wider3VecV4units5Int32VvgZ"},
		{"a static addressor", Addressor, "unit", "$s5Wider3VecV4units5Int32Vvau"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := vec
			d.Name = c.prop
			d.Signature = sig(nil, i32)
			got, err := c.make(d)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

// TestGenericFunction holds the mangler to swiftc's own symbols for
// generic declarations. A generic parameter has no name in a symbol:
// it is its place in the signature that declared it.
func TestGenericFunction(t *testing.T) {
	i32 := types.Typ[types.Int32]
	tp := func(name string) *types.TypeParam { return &types.TypeParam{Name: name} }

	for _, c := range []struct {
		name string
		decl Decl
		want string
	}{
		{"one parameter, used as an argument",
			func() Decl {
				a := tp("T")
				return Decl{Module: "m", Name: "one", Signature: &types.Signature{
					TypeParams: []*types.TypeParam{a},
					Params:     []*types.Param{{Type: a}},
					Results:    i32,
				}}
			}(),
			"$s1m3oneys5Int32VxlF"},

		{"one parameter, also the result",
			func() Decl {
				a := tp("T")
				return Decl{Module: "m", Name: "ret", Signature: &types.Signature{
					TypeParams: []*types.TypeParam{a},
					Params:     []*types.Param{{Type: a}},
					Results:    a,
				}}
			}(),
			"$s1m3retyxxlF"},

		{"two parameters",
			func() Decl {
				a, b := tp("T"), tp("U")
				return Decl{Module: "m", Name: "two", Signature: &types.Signature{
					TypeParams: []*types.TypeParam{a, b},
					Params:     []*types.Param{{Type: a}, {Type: b}},
					Results:    i32,
				}}
			}(),
			"$s1m3twoys5Int32Vx_q_tr0_lF"},

		{"three parameters",
			func() Decl {
				a, b, c := tp("A"), tp("B"), tp("C")
				return Decl{Module: "m", Name: "three", Signature: &types.Signature{
					TypeParams: []*types.TypeParam{a, b, c},
					Params:     []*types.Param{{Type: a}, {Type: b}, {Type: c}},
					Results:    i32,
				}}
			}(),
			"$s1m5threeys5Int32Vx_q_q0_tr1_lF"},

		{"a concrete parameter beside a generic one",
			func() Decl {
				a := tp("T")
				return Decl{Module: "m", Name: "mixed", Signature: &types.Signature{
					TypeParams: []*types.TypeParam{a},
					Params:     []*types.Param{{Type: i32}, {Type: a}},
					Results:    a,
				}}
			}(),
			"$s1m5mixedyxs5Int32V_xtlF"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := Function(c.decl)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("\ngot  %s\nwant %s", got, c.want)
			}
		})
	}
}
