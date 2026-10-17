// A stored property's default, with no type written, calls a function
// declared after the type.
final class C {
    var xs = seed()
    var n = seed().count
}
struct S {
    var name = label()
}
func seed() -> [Int] { [1, 2, 3] }
func label() -> String { "s" }
print(C().xs, C().n, S().name)
