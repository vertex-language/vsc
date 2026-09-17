package types

import "testing"

// TestVertexNamesDenoteTheSameType is the guard the comment on vertexNames
// promises: every Vertex spelling it hands out must be a real alias for the
// Swift name it was given for, so the two tables cannot drift apart.
func TestVertexNamesDenoteTheSameType(t *testing.T) {
	for swift, vertex := range vertexNames {
		want, ok := swiftTypes[swift]
		if !ok {
			t.Errorf("vertexNames has %q, which is not a universe name", swift)
			continue
		}
		got, ok := vertexTypes[vertex]
		if !ok {
			t.Errorf("%s is spelled %q, which is not a Vertex alias", swift, vertex)
			continue
		}
		if !Identical(got, want) {
			t.Errorf("%s is spelled %q, which denotes %s", swift, vertex, got)
		}
	}
}

// TestVertexNameCoversTheScalars: every lowercase alias should have a Swift
// name that maps back to one, or an interface would print a name the
// language spells differently with nothing to say so.
func TestVertexNameCoversTheScalars(t *testing.T) {
	for vertex, typ := range vertexTypes {
		b, ok := typ.(*Basic)
		if !ok {
			continue // `any` is an existential, and has no Basic name
		}
		name, ok := VertexName(b.Name())
		if !ok {
			t.Errorf("%s is spelled %q in Vertex, but VertexName(%q) says nothing",
				b.Name(), vertex, b.Name())
			continue
		}
		if _, ok := vertexTypes[name]; !ok {
			t.Errorf("VertexName(%q) = %q, which is not a Vertex alias", b.Name(), name)
		}
	}
}

// TestVertexNameLeavesOtherNamesAlone: a type the language spells one way
// is not rewritten, and neither is anything that merely looks like one.
func TestVertexNameLeavesOtherNamesAlone(t *testing.T) {
	for _, name := range []string{"Error", "ArraySlice", "MyInt32", "Int32Extra", "", "int32"} {
		if got, ok := VertexName(name); ok {
			t.Errorf("VertexName(%q) = %q, want nothing", name, got)
		}
	}
}
