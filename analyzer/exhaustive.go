package analyzer

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

// Switch exhaustiveness, as swiftc's space engine decides it: each case
// pattern without a where clause is the space of values it covers, and the
// switch is exhaustive when no value is left outside all of them.
//
// A space is a wildcard (nil), a constructor applied to the spaces of its
// arguments -- true, false, an enum case and its payload, Optional's some
// and none, a tuple -- or opaque: a value such as a number or a string that
// matches some values of a type whose values cannot be listed, and so
// never finishes covering it.

type space struct {
	ctor string // "" is opaque
	args []*space
}

var opaque = &space{}

// tupleCtor is the one constructor of a tuple type.
const tupleCtor = "()"

// ctor is a constructor of a type and the types of its arguments.
type ctor struct {
	name string
	args []types.Type
}

// ctorsOf lists a type's constructors. finite is false for a type whose
// values cannot be listed (Int, String, a struct); known is false for one
// this cannot tell about, such as a type parameter.
func ctorsOf(t types.Type) (cs []ctor, finite, known bool) {
	if t == nil {
		return nil, false, false
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Kind() == types.Bool {
			return []ctor{{name: "true"}, {name: "false"}}, true, true
		}
		return nil, false, true
	case *types.Optional:
		return []ctor{{name: "some", args: []types.Type{u.Wrapped}}, {name: "none"}}, true, true
	case *types.Tuple:
		args := make([]types.Type, len(u.Elements))
		for i, e := range u.Elements {
			args[i] = e.Type
		}
		return []ctor{{name: tupleCtor, args: args}}, true, true
	case *types.Enum:
		if len(u.TypeParams) > 0 {
			return nil, false, false
		}
		for _, ec := range u.Cases {
			cs = append(cs, ctor{name: ec.Name, args: payloadTypes(ec)})
		}
		return cs, true, true
	case *types.TypeParam:
		return nil, false, false
	}
	return nil, false, true
}

// payloadTypes is what an enum case carries, one type per value.
func payloadTypes(ec *types.EnumCase) []types.Type {
	if ec.AssociatedType == nil {
		return nil
	}
	if tu, ok := ec.AssociatedType.(*types.Tuple); ok && ec.Label == "" {
		out := make([]types.Type, len(tu.Elements))
		for i, e := range tu.Elements {
			out[i] = e.Type
		}
		return out
	}
	return []types.Type{ec.AssociatedType}
}

// spaceOf is the space a case pattern covers in a value of type t.
func (c *checker) spaceOf(p ast.Pattern, t types.Type) *space {
	switch p := p.(type) {
	case nil, *ast.WildcardPattern, *ast.IdentPattern:
		return nil
	case *ast.ValueBindingPattern:
		return c.spaceOf(p.Pat, t)
	case *ast.TypedPattern:
		return c.spaceOf(p.Pat, t)
	case *ast.TuplePattern:
		if len(p.Elems) == 1 && p.Elems[0].Label == nil {
			return c.spaceOf(p.Elems[0].Pat, t)
		}
		tu, ok := underlying(t).(*types.Tuple)
		if !ok || len(tu.Elements) != len(p.Elems) {
			return opaque
		}
		args := make([]*space, len(p.Elems))
		for i, e := range p.Elems {
			args[i] = c.spaceOf(e.Pat, tu.Elements[i].Type)
		}
		return &space{ctor: tupleCtor, args: args}
	case *ast.OptionalPattern:
		o, ok := underlying(t).(*types.Optional)
		if !ok {
			return opaque
		}
		return &space{ctor: "some", args: []*space{c.spaceOf(p.Pat, o.Wrapped)}}
	case *ast.EnumCasePattern:
		if p.Name == nil {
			return opaque
		}
		return c.caseSpace(p.Name.Text(c.file), p.Args, t)
	case *ast.ExprPattern:
		switch x := p.X.(type) {
		case *ast.BasicLit:
			switch x.Kind {
			case token.TRUE, token.FALSE:
				if b, ok := underlying(t).(*types.Basic); ok && b.Kind() == types.Bool {
					if x.Kind == token.TRUE {
						return &space{ctor: "true"}
					}
					return &space{ctor: "false"}
				}
			case token.NIL:
				if _, ok := underlying(t).(*types.Optional); ok {
					return &space{ctor: "none"}
				}
			}
		case *ast.MemberExpr:
			if x.Name != nil {
				return c.caseSpace(x.Name.Text(c.file), nil, t)
			}
		case *ast.ParenExpr:
			return c.spaceOf(&ast.ExprPattern{X: x.X}, t)
		case *ast.OptionalExpr:
			// `true?` in a pattern is .some(true).
			if o, ok := underlying(t).(*types.Optional); ok {
				return &space{ctor: "some", args: []*space{c.spaceOf(&ast.ExprPattern{X: x.X}, o.Wrapped)}}
			}
		}
	}
	return opaque
}

// caseSpace is `.name(args)` matched against t: a case of an enum, of an
// Optional, or -- where t is an Optional of an enum -- of what it wraps.
func (c *checker) caseSpace(name string, args *ast.TuplePattern, t types.Type) *space {
	switch u := underlying(t).(type) {
	case *types.Optional:
		switch name {
		case "none":
			return &space{ctor: "none"}
		case "some":
			if args == nil {
				return &space{ctor: "some", args: []*space{nil}}
			}
			return &space{ctor: "some", args: []*space{c.payloadSpace(args, []types.Type{u.Wrapped})}}
		}
		return &space{ctor: "some", args: []*space{c.caseSpace(name, args, u.Wrapped)}}
	case *types.Enum:
		for _, ec := range u.Cases {
			if ec.Name != name {
				continue
			}
			payload := payloadTypes(ec)
			s := &space{ctor: name, args: make([]*space, len(payload))}
			if args == nil {
				return s
			}
			if len(args.Elems) == len(payload) {
				for i, e := range args.Elems {
					s.args[i] = c.spaceOf(e.Pat, payload[i])
				}
				return s
			}
			// One pattern for several values -- `.a(let pair)` -- is all
			// of them only where it matches anything.
			if len(args.Elems) == 1 && c.spaceOf(args.Elems[0].Pat, nil) == nil {
				return s
			}
			return opaque
		}
	}
	return opaque
}

// payloadSpace is the one value `.some(x)` carries.
func (c *checker) payloadSpace(args *ast.TuplePattern, ts []types.Type) *space {
	if len(args.Elems) != 1 {
		return opaque
	}
	return c.spaceOf(args.Elems[0].Pat, ts[0])
}

func underlying(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	return t.Underlying()
}

// uncovered reports whether some value of the types ts escapes every row.
// budget bounds the search: where it runs out, the switch is taken to be
// exhaustive, which never rejects a program Swift accepts.
func uncovered(rows [][]*space, ts []types.Type, budget *int) bool {
	*budget--
	if *budget < 0 {
		return false
	}
	if len(ts) == 0 {
		return len(rows) == 0
	}
	cs, finite, known := ctorsOf(ts[0])
	seen := map[string]bool{}
	for _, r := range rows {
		if h := r[0]; h != nil && h.ctor != "" {
			seen[h.ctor] = true
		}
	}
	complete := finite && len(cs) > 0
	for _, k := range cs {
		complete = complete && seen[k.name]
	}
	if complete {
		for _, k := range cs {
			next := append(append([]types.Type(nil), k.args...), ts[1:]...)
			if uncovered(specialize(rows, k), next, budget) {
				return true
			}
		}
		return false
	}
	if !known && len(seen) > 0 {
		return false
	}
	var rest [][]*space
	for _, r := range rows {
		if r[0] == nil {
			rest = append(rest, r[1:])
		}
	}
	return uncovered(rest, ts[1:], budget)
}

// specialize is the rows that match constructor k, with k's arguments in
// place of the first column.
func specialize(rows [][]*space, k ctor) [][]*space {
	var out [][]*space
	for _, r := range rows {
		h := r[0]
		if h != nil && h.ctor != k.name {
			continue
		}
		row := make([]*space, len(k.args), len(k.args)+len(r)-1)
		if h != nil {
			copy(row, h.args)
		}
		out = append(out, append(row, r[1:]...))
	}
	return out
}

// missingSpaces reports whether a switch over t whose cases cover rows
// leaves something out, and for an enum, which cases.
func missingSpaces(rows [][]*space, t types.Type) (bool, []string) {
	budget := 100000
	if !uncovered(rows, []types.Type{t}, &budget) {
		return false, nil
	}
	var missing []string
	if e, ok := underlying(t).(*types.Enum); ok {
		cs, _, _ := ctorsOf(e)
		for _, k := range cs {
			b := 100000
			if uncovered(specialize(rows, k), k.args, &b) {
				missing = append(missing, "."+k.name)
			}
		}
	}
	return true, missing
}
