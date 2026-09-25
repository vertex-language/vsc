// Float16 as a built-in type: arithmetic rounds once per operation,
// comparisons, conversions both ways, rounding, special values, bit
// patterns, and Float16 in generic code and collections.
let a: Float16 = 1.5
let b: Float16 = 0.1
print(a + b, a - b, a * b, a / b, -a, a + b * 2)
print(a == 1.5, a != b, a < b, a <= a, b > a, a >= b)
print(Float(b), Double(b), Int(a * 3), Int16(Float16(-7.9)), UInt8(Float16(200.4)))
print(Float16(Float(0.3)), Float16(Double(1e-6)), Float16(12345), Float16(Int32(-3)))
let c: Float16 = 2.5
print(c.rounded(), c.rounded(.toNearestOrEven), c.rounded(.down), c.rounded(.up), (-c).rounded(.towardZero))
print(c.squareRoot(), c.magnitude, (-c).magnitude, abs(-c), c.isNaN, c.isFinite)
let big: Float16 = 60000
print(big * 2, (big * 2).isInfinite, Float16.nan.isNaN, Float16.infinity > big, Float16.leastNonzeroMagnitude)
print(a.bitPattern, Float16(bitPattern: 0x3C00), Float16.greatestFiniteMagnitude.bitPattern)
print(c.nextUp, c.nextDown, c.ulp, Float16.ulpOfOne)

func total<T: FloatingPoint>(_ xs: [T]) -> T {
    var s: T = 0
    for x in xs { s += x }
    return s
}
let xs: [Float16] = [0.1, 0.2, 0.3, 1000]
print(total(xs), xs.max()!, xs.min()!, xs.sorted(by: >))
var m = [Float16](repeating: 0.5, count: 3)
m[1] *= 3
print(m, m.reduce(0, +))
