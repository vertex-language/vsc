// Remainders, square roots and the parts of a Double.
let x = 10.5
print(x.truncatingRemainder(dividingBy: 3), x.remainder(dividingBy: 3))
print(x.squareRoot(), (2.0).squareRoot(), abs(-3.25), x.sign == .plus)
print(x.exponent, x.significand, (-x).magnitude, Double.ulpOfOne)
