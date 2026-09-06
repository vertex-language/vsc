// Two names moving through a hundred allocations, each keeping what it points at alive.
final class Box { var n: Int32 = 0 }
func main() -> Int32 {
  var cur = Box(); cur.n = 1
  var last = cur
  for i in 1..<100 { let nb = Box(); nb.n = Int32(i); last = cur; cur = nb }
  return cur.n - last.n + 42 - 1
}
