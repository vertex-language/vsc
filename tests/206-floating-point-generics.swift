// Code written once over FloatingPoint, at Float and Double.
func mean<T: FloatingPoint>(_ xs: [T]) -> T {
    xs.isEmpty ? .nan : xs.reduce(0, +) / T(xs.count)
}
func hypotenuse<T: BinaryFloatingPoint>(_ a: T, _ b: T) -> T { (a * a + b * b).squareRoot() }
print(mean([1.0, 2.0, 4.0]), mean([Float(1), 2]), mean([Double]()).isNaN)
print(hypotenuse(3.0, 4.0), hypotenuse(Float(5), 12))
print(Double.pi.rounded(.down), (7.5 as Float).nextUp > 7.5)
