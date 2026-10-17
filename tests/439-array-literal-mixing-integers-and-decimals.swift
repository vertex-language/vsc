// An array literal mixing integer and decimal literals is a [Double].
for k in 0..<4 { print(Float([1, 0.7, 0, -0.7][k])) }
let i = 1
print(Float([0, 1.8, 0][i]))
let xs = [2, 0.5]
print(xs, type(of: xs))
