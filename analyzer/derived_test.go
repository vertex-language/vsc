package analyzer

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/ast"
)

// TestDerivedConformances holds the checker to the members swiftc derives:
// a type's `==`, `hash(into:)` and allCases, where it conforms -- itself
// or through a protocol that refines the one derived -- and has none.
func TestDerivedConformances(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want []string // lines the derived file holds; none at all for nil
	}{
		{"an Equatable struct compares its stored properties",
			"struct P: Equatable { var x: Int; let y: Int; var sum: Int { return x + y }; static var zero = 0 }",
			[]string{"extension P {", "static func __derived_struct_equals(_ a: P, _ b: P) -> Bool {",
				"if !(a.x == b.x) { return false }", "if !(a.y == b.y) { return false }"}},

		{"Comparable and Hashable are Equatable too, and a public type's is public",
			"public struct S: Comparable { let n: Int; public static func < (a: S, b: S) -> Bool { return a.n < b.n } }",
			[]string{"public static func __derived_struct_equals(_ a: S, _ b: S) -> Bool {"}},

		{"so is a protocol of the program's own that refines Equatable",
			"protocol Shape: Equatable {}\nstruct Sq: Shape { let side: Int }",
			[]string{"extension Sq {", "if !(a.side == b.side) { return false }"}},

		{"and an OptionSet",
			"struct F: OptionSet { let rawValue: UInt8 }",
			[]string{"static func __derived_struct_equals(_ a: F, _ b: F) -> Bool {", "if !(a.rawValue == b.rawValue)"}},

		{"a struct with no stored properties is always equal",
			"struct E: Hashable {}",
			[]string{"return true"}},

		{"a conformance added by an extension counts",
			"struct Q { let n: Int }\nextension Q: Equatable {}",
			[]string{"extension Q {"}},

		{"a nested struct is named through its outer type",
			"enum Outer { struct Inner: Equatable { let n: Int } }",
			[]string{"extension Outer.Inner {"}},

		{"a package clause is repeated",
			"package time\npublic struct D: Equatable { let n: Int }",
			[]string{"package time"}},

		{"a struct that writes its own == gets nothing",
			"struct L: Equatable { let n: Int; static func == (a: L, b: L) -> Bool { return true } }",
			nil},

		{"nor one whose == is at the top level",
			"struct M: Equatable { let n: Int }\nfunc == (a: M, b: M) -> Bool { return true }",
			nil},

		{"a generic struct's == is where its parameters are Equatable",
			"struct Box<T>: Equatable { let v: T }",
			[]string{"extension Box where T: Equatable {",
				"static func __derived_struct_equals(_ a: Box<T>, _ b: Box<T>) -> Bool {",
				"if !(a.v == b.v) { return false }"}},

		{"a Hashable struct hashes its stored properties",
			"struct H: Hashable { let a: Int; var b: String }",
			[]string{"func hash(into hasher: inout Hasher) {", "hasher.combine(self.a)", "hasher.combine(self.b)",
				"static func __derived_struct_equals(_ a: H, _ b: H) -> Bool {"}},

		{"a Hashable enum of cases hashes its case, and needs no ==",
			"public enum C: Hashable { case red, green }",
			[]string{"public func hash(into hasher: inout Hasher) {", "case .red: hasher.combine(0)", "case .green: hasher.combine(1)"}},

		{"one that writes its own hash(into:) gets only ==",
			"struct W: Hashable { let n: Int; func hash(into hasher: inout Hasher) { } }",
			[]string{"__derived_struct_equals"}},

		{"an enum that carries values compares and hashes case by case",
			"enum P: Hashable { case a(Int), b, c(String, Int) }",
			[]string{"static func __derived_enum_equals(_ a: P, _ b: P) -> Bool {",
				"case .a(let l0):", "if case .a(let r0) = b { return l0 == r0 }",
				"if case .c(let r0, let r1) = b { return l0 == r0 && l1 == r1 }",
				"case .b:", "if case .b = b { return true }",
				"case .c(let v0, let v1):", "hasher.combine(2)", "hasher.combine(v1)"}},

		{"an enum of cases alone is Hashable without saying so",
			"enum D { case up, down }",
			[]string{"extension D: Hashable {", "case .up: hasher.combine(0)"}},

		{"and CaseIterable lists its cases",
			"enum K: CaseIterable { case a, b }",
			[]string{"static var allCases: [K] { [.a, .b] }"}},

		{"but not one that already hashes",
			"enum D: Hashable { case up }\nextension D { func hash(into hasher: inout Hasher) { } }",
			nil},

		{"nor one that is not Equatable",
			"struct Plain { let n: Int }",
			nil},
	} {
		file, _ := parseSnippet(t, c.src)
		info, diags := Check([]*ast.File{file})
		got := string(info.DerivedText)
		if c.want == nil {
			if got != "" {
				t.Errorf("%s: derived\n%s", c.name, got)
			}
			continue
		}
		for _, line := range c.want {
			if !strings.Contains(got, line) {
				t.Errorf("%s: no %q in\n%s", c.name, line, got)
			}
		}
		for _, d := range diags {
			t.Errorf("%s: %s", c.name, d.Message)
		}
	}
}
