// A literal takes the type its context asks for.
let d: Double = 3
let f: Float = 2
let u: UInt8 = 255
let x = 3 + 0.5
print(d / 2, f / 4, u, x, type(of: x), type(of: 3), type(of: u))
