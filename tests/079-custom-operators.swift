// Declaring prefix, postfix and infix operators of a program's own.
prefix operator √
postfix operator %%
infix operator **: MultiplicationPrecedence
prefix func √ (x: Double) -> Double { x.squareRoot() }
postfix func %% (x: Int) -> Double { Double(x) / 100 }
func ** (base: Int, exp: Int) -> Int {
    var r = 1
    for _ in 0..<exp { r *= base }
    return r
}
print(√16.0, 25%%, 2 ** 10, 3 + 2 ** 3)
