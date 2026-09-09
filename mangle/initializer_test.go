package mangle

import (
	"testing"

	"github.com/vertex-language/vsc/types"
)

// TestInitializerSymbols: an initializer's symbol is not a function's.
//
// It has no name -- `init` is a keyword, not an identifier -- so
// nothing is written between the type and the signature, and it ends
// in `cfC` where a function ends in `F`: the type written out in
// full, then the allocating entry point, which is what a call to
// `P(v: 3)` reaches.
//
// Every name below is swiftc's, read out of its own SIL for the same
// three declarations.
func TestInitializerSymbols(t *testing.T) {
	pStruct := &types.Struct{Name: "P"}
	p := types.NewNamed("P", "init2", pStruct)
	i32 := types.Typ[types.Int32]

	cases := []struct {
		name string
		sig  *types.Signature
		want string
	}{
		{"one labelled parameter",
			&types.Signature{Params: []*types.Param{{Name: "v", Label: "v", Type: i32}}, Results: p},
			"$s5init21PV1vACs5Int32V_tcfC"},
		{"no parameters",
			&types.Signature{Results: p},
			"$s5init21PVACycfC"},
		{"two unlabelled parameters",
			&types.Signature{Params: []*types.Param{
				{Name: "a", Label: "_", Type: i32},
				{Name: "b", Label: "_", Type: i32},
			}, Results: p},
			"$s5init21PVyACs5Int32V_AEtcfC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Initializer(Decl{
				Module:    "init2",
				Context:   []Nominal{{Name: "P", Kind: Struct}},
				Signature: tc.sig,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}
