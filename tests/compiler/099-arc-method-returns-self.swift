// A method that returns its own receiver.
final class B { var n: Int32 = 0
  func set(_ v: Int32) -> B { n = v; return self } }
func main() -> Int32 { let b = B(); return b.set(42).n }
