// A reference mutated by a bottom-tested loop.
final class Acc { var n: Int32 = 0 }
func main() -> Int32 {
  let a = Acc()
  var i: Int32 = 0
  repeat { a.n = a.n + i; i = i + 1 } while i < 10
  return a.n - 3
}
