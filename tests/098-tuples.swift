// Tuples: positional and named elements, destructuring, and as results.
func minMax(_ xs: [Int]) -> (min: Int, max: Int) {
    var lo = xs[0], hi = xs[0]
    for x in xs { lo = Swift.min(lo, x); hi = Swift.max(hi, x) }
    return (lo, hi)
}
let r = minMax([4, -2, 9, 3])
print(r.min, r.max, r.0, r.1, r)
let (first, _) = r
var point = (x: 1, y: 2)
point.x += 10
print(first, point, point == (11, 2))
