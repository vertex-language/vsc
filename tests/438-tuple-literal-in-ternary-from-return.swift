// A tuple literal in one arm of ?: takes its element types from the
// optional-tuple return type.
final class Thing {
    let n: Int
    init(_ n: Int) { self.n = n }
}
func nearest(_ x: [Thing]) -> (Thing, Float)? {
    return x.isEmpty ? nil : (x[0], 1)
}
func sized(_ k: Int) -> (UInt8, Double) {
    return k > 0 ? (1, 2) : (0, 0.5)
}
print(nearest([])  == nil, nearest([Thing(4)])!.0.n, nearest([Thing(4)])!.1)
print(sized(1), sized(0))
