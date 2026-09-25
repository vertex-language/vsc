// An array literal appended with +=: its elements are the array's type,
// a [UInt8]'s bytes and a [Double]'s doubles, as with +.
var a: [UInt8] = [1]
a += [2, 0x80]
var b: [Double] = []
b += [1, 2.5]
let c = a + [7]
print(a, b, c)
