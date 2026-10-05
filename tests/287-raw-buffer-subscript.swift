// A raw buffer pointer read by subscript: the bytes of an array and of an
// integer, through withUnsafeBytes.
let xs: [UInt16] = [0x0102, 0x0304]
xs.withUnsafeBytes { raw in
    print(raw.count, raw[0], raw[1], raw[2], raw[3])
    var sum = 0
    for i in 0..<raw.count { sum += Int(raw[i]) }
    print(sum)
}
let n: UInt32 = 0xAABBCCDD
withUnsafeBytes(of: n) { raw in print(raw[0], raw[3]) }
