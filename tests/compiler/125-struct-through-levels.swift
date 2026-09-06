// A struct copied through eight calls.
struct S { var a: Int32; var b: Int32 }
func one(_ s: S) -> S { return S(a: s.a + 1, b: s.b + 1) }
func two(_ s: S) -> S { return one(one(s)) }
func three(_ s: S) -> S { return two(two(s)) }
func main() -> Int32 { let r = three(S(a: 0, b: 30)); return r.a + r.b }
