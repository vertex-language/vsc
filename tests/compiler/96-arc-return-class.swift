// A reference handed back from a call belongs to the caller.
final class Box { var n: Int32 = 0 }
func make(_ v: Int32) -> Box { let b = Box(); b.n = v; return b }
func main() -> Int32 { let a = make(20); let b = make(22); return a.n + b.n }
