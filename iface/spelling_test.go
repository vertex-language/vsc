package iface

import "testing"

// TestVertexTypesRewritesTheUniverse: an interface is Vertex source, so the
// types in it are spelled the way Vertex spells them -- including inside a
// composite, where only the universe's own names change.
func TestVertexTypesRewritesTheUniverse(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Int32", "int32"},
		{"String", "string"},
		{"[UInt8]", "[uint8]"},
		{"ArraySlice<UInt8>", "ArraySlice<uint8>"},
		{"(code: Int32, message: String)", "(code: int32, message: string)"},
		{"[String: [Int]]", "[string: [int]]"},
		{"Optional<Bool>", "Optional<bool>"},
		{"(Int32, String) -> Bool", "(int32, string) -> bool"},
		{"UnsafeMutablePointer<Int32>?", "UnsafeMutablePointer<int32>?"},
		{"Float", "float32"},
		{"Double", "float64"},

		// A name that is not the universe's is left as it was, and so is
		// one that merely contains a universe name.
		{"TcpStream", "TcpStream"},
		{"MyInt32", "MyInt32"},
		{"Int32Extra", "Int32Extra"},
		{"Error", "Error"},
		{"[TcpError]", "[TcpError]"},
		{"", ""},
	}
	for _, c := range cases {
		if got := vertexTypes(c.in); got != c.want {
			t.Errorf("vertexTypes(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
