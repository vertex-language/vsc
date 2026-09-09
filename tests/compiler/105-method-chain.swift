// A method called on what a method returned, on a value type.
struct V { var n: Int32
  func plus(_ o: V) -> V { return V(n: n + o.n) }
  func times(_ k: Int32) -> V { return V(n: n * k) } }
func main() -> Int32 { return V(n: 3).plus(V(n: 4)).times(6).n }
