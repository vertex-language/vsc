// A range subscript of an array is an ArraySlice: the array and bounds
// checked against its count. A slice counts what lies between them, and
// lends its bytes from its first element on.
@_silgen_name("strlen")
func c_strlen(_ s: UnsafeRawPointer) -> Int

func weigh(_ s: ArraySlice<UInt8>) -> Int {
    if s.isEmpty {
        return 1000
    }
    return s.withUnsafeBytes { raw in raw.count * 10 + c_strlen(raw.baseAddress!) }
}

func main() -> Int32 {
    let bytes: [UInt8] = [104, 105, 0, 119, 111, 114, 108, 100, 0]
    var total = weigh(bytes[0..<3])
    total += weigh(bytes[3...8])
    total += weigh(bytes[2..<2])
    let part = bytes[3..<5]
    total += part.count * 100
    return Int32(total % 251)
}
