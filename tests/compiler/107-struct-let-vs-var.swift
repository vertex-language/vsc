// A struct bound to a let is a copy, and writing the original leaves it alone.
struct S { var n: Int32 }
func main() -> Int32 {
  var a = S(n: 1)
  let b = a
  a.n = 41
  return a.n + b.n
}
