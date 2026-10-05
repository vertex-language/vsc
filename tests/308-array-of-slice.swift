// [T](slice): an array made from an ArraySlice with the array type
// written out.
let xs: [UInt8] = [1, 2, 3, 4, 5]
let ys = [UInt8](xs[1..<4])
print(ys)
let zs = [Int](xs.map { Int($0) }[2...])
print(zs)
