// A reference reached through two other references.
final class C { var n: Int32 = 42 }
final class B { var c = C() }
final class A { var b = B() }
func main() -> Int32 { let a = A(); return a.b.c.n }
