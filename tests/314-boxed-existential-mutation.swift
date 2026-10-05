// A mutating requirement called through a copy of an existential whose
// value is boxed: the copy is changed, and the original is not, as a
// value's copies are (rung 292 boxes; this writes through the box).
protocol P { var total: Int { get }; mutating func bump(_ by: Int) }
struct Four: P {
    var a = 1, b = 2, c = 3, d = 4
    var total: Int { a + b + c + d }
    mutating func bump(_ by: Int) { a += by }
}
struct Six: P {
    var a = 1, b = 2, c = 3, d = 4, e = 5, f = 6
    var total: Int { a + b + c + d + e + f }
    mutating func bump(_ by: Int) { f += by }
}
var x: any P = Four()
x.bump(10)
var y = x
y.bump(1)
print(x.total, y.total)
var xs: [any P] = [Six(), Six()]
var ys = xs
ys[0].bump(100)
print(xs.map { $0.total }, ys.map { $0.total })
