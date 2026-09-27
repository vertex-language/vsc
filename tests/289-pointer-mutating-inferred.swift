// UnsafeMutablePointer(mutating:) with its element type taken from the
// pointer it is given (rung 272 spells it out).
var v: Int32 = 5
withUnsafePointer(to: &v) { p in
    let m = UnsafeMutablePointer(mutating: p)
    m.pointee += 1
}
print(v)
