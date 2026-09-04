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
	intT := types.Typ[types.Int]
	for _, c := range []struct {
		name string
		decl Decl
		err  error
	}{
		{"generic function",
			Decl{Module: "m", Name: "f", Signature: &types.Signature{
				TypeParams: []*types.TypeParam{{Name: "T"}},
			}}, ErrUnsupported},
		{"async function",
			Decl{Module: "m", Name: "f", Signature: &types.Signature{Async: true}}, ErrUnsupported},
		{"variadic parameter",
			Decl{Module: "m", Name: "f", Signature: sig(
				[]*types.Param{{Type: intT, Variadic: true}}, nil)}, ErrUnsupported},
		{"a protocol",
			Decl{Module: "m", Name: "f", Signature: sig(
				[]*types.Param{p("", &types.Protocol{Name: "P"})}, nil)}, ErrUnsupported},
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
