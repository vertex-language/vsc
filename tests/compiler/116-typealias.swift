// A typealias is another spelling, and a literal fits it the same way.
typealias Num = Int32
func add(_ a: Num, _ b: Num) -> Num { return a + b }
func main() -> Int32 { return add(20, 22) }
