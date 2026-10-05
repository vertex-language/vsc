// How a Double prints across its range: the shortest digits that round-trip.
let values: [Double] = [1, 0.5, 100, 1e15, 1e16, 1.0e-4, 1.0e-5, 123.456, 2.0 / 3.0, 5e-324, 1.7976931348623157e308]
for v in values { print(v, -v) }
print(Float(1) / 3, Float(1e10), Float(1e-10))
