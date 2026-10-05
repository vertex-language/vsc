// addingProduct is a fused multiply-add: self + lhs·rhs rounded once,
// which differs from the product rounded and then added.
let a: Float = 0.1
print(a.addingProduct(0.2, 0.3), a + 0.2 * 0.3)
var d: Double = 1
d.addProduct(1e-16, 1e-16)
print(d == 1, (0.1 as Double).addingProduct(0.2, 0.3))
// x·x - x·x is exactly the rounding error of x·x when fused.
let x: Float = 1.1
print((-(x * x)).addingProduct(x, x))
