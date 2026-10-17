// A tuple literal returned where an optional tuple is wanted takes its
// element types from it, and nil returns are the optional tuple's.
func f(_ n: Int) -> (Bool, UInt64)? {
    if n < 0 { return nil }
    return (true, 1)
}
print(f(1)!.1 + 1, f(-1) == nil)
func parse(_ s: String) -> (negative: Bool, magnitude: UInt64)? {
    guard let first = s.first else { return nil }
    if first == "-" {
        guard let m = UInt64(s.dropFirst()) else { return nil }
        return (true, m)
    }
    guard let m = UInt64(s) else { return nil }
    return (false, m)
}
for s in ["12", "-7", "", "x"] {
    if let p = parse(s) { print(p.negative, p.magnitude) } else { print("nil") }
}
