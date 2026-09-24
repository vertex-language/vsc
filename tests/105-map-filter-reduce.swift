// map, filter and reduce, alone and chained.
let xs = Array(1...10)
let squares = xs.map { $0 * $0 }
let evens = xs.filter { $0 % 2 == 0 }
let sum = xs.reduce(0, +)
let text = xs.reduce(into: "") { $0 += String($1) }
print(squares, evens, sum, text)
print(xs.filter { $0 % 3 == 0 }.map { $0 * 10 }.reduce(0, +))
