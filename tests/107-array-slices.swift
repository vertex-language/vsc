// ArraySlice shares indices with its base, and converts back to an Array.
let xs = [10, 20, 30, 40, 50]
let mid = xs[1..<4]
print(mid, mid.count, mid.startIndex, mid[2])
print(Array(mid), xs[..<2], xs[3...], xs.prefix(2), xs.suffix(1))
var copy = Array(xs[2...])
copy[0] = 0
print(copy, xs)
