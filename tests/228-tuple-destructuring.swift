// Tuples taken apart in declarations, loops, closures and assignments.
let (q, r) = 17.quotientAndRemainder(dividingBy: 5)
var (a, b) = (1, 2)
(a, b) = (b, a)
print(q, r, a, b)
let points = [(x: 1, y: 2), (x: 3, y: 4)]
for (x, y) in points { print(x * y) }
print(points.map { p in p.x + p.y }, points.map { $0.y })
let ((m, n), s) = ((1, 2), "s")
print(m, n, s)
