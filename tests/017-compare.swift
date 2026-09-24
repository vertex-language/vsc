// The six comparisons on signed and unsigned integers.
for (a, b) in [(1, 2), (2, 1), (3, 3), (-1, 1)] {
    print(a < b, a <= b, a > b, a >= b, a == b, a != b)
}
let u: UInt = 1
let v: UInt = .max
print(u < v, u > v)
