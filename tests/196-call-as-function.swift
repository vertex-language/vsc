// callAsFunction lets a value be called like a function.
struct Polynomial {
    let coefficients: [Int]
    func callAsFunction(_ x: Int) -> Int {
        coefficients.reversed().reduce(0) { $0 * x + $1 }
    }
    func callAsFunction(derivativeAt x: Int) -> Int {
        let d = Polynomial(coefficients: coefficients.enumerated().dropFirst().map { $0.offset * $0.element })
        return d(x)
    }
}
let p = Polynomial(coefficients: [1, 2, 3])
print(p(0), p(2), p(derivativeAt: 2), [0, 1, 2].map { p($0) })
