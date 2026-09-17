// A reference passed in and stored outlives the call it came through.
final class Box { var n: Int32 = 0 }
final class Holder { var b = Box() }
func put(_ h: Holder, _ b: Box) { h.b = b }
func main() -> Int32 { let h = Holder(); let x = Box(); x.n = 42; put(h, x); return h.b.n }
