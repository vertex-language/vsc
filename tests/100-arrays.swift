// Array: literals, count, subscripts, append and emptiness.
var xs = [10, 20, 30]
print(xs, xs.count, xs[0], xs[2], xs.isEmpty)
xs.append(40)
xs[1] = 21
print(xs, xs.first as Any, xs.last as Any)
let empty: [String] = []
print(empty, empty.isEmpty, empty.first as Any, Array(repeating: 0, count: 3))
