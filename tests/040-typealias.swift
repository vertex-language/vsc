// A typealias is another name for the same type.
typealias Byte = UInt8
typealias Pair = (x: Int, y: Int)
typealias Transform = (Int) -> Int
let b: Byte = 200
let p: Pair = (3, 4)
let double: Transform = { $0 * 2 }
print(b, p.x + p.y, double(21), Byte.max)
