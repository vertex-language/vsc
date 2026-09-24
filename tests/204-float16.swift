// Float16: its range, its rounding, and conversion to and from Float.
let h: Float16 = 1.0 / 3.0
print(h, Float(h), Float16.greatestFiniteMagnitude, Float16.leastNormalMagnitude)
let big = Float16(Float(70000))
print(big, big.isInfinite, Float16(2049), Float16(0.1) + Float16(0.2))
