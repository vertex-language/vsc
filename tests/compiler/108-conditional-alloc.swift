// A reference built on one arm of a conditional.
final class B { var n: Int32 = 0 }
func pick(_ f: Bool) -> B { let b = B(); b.n = f ? 42 : 1; return b }
func main() -> Int32 { return pick(true).n }
