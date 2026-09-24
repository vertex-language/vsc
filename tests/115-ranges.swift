// Ranges as values: contains, count, bounds, clamping and overlaps.
let r = 3..<8
let c = 3...8
print(r.contains(8), c.contains(8), r.count, c.count, r.lowerBound, c.upperBound)
print(r.isEmpty, (5..<5).isEmpty, (0..<10).clamped(to: 4..<20), r.overlaps(7..<9))
let chars = "a"..."f"
print(chars.contains("c"), chars.contains("z"))
print(Array((1...10).reversed().prefix(3)))
