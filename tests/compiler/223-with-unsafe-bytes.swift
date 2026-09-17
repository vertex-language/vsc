// An array's bytes can be lent to a closure as an UnsafeRawBufferPointer:
// where the elements start, and how many bytes they are -- the count times
// the elements' stride. What the closure returns is what the call does,
// and C can read the bytes through the base address.
@_silgen_name("strlen")
func c_strlen(_ s: UnsafeRawPointer) -> Int

func main() -> Int32 {
    let text: [UInt8] = [104, 101, 108, 108, 111, 33, 0]
    let wide: [Int32] = [1, 2, 3, 4]
    let empty: [Int32] = []
    var total = text.withUnsafeBytes { raw in
        c_strlen(raw.baseAddress!)
    }
    total += wide.withUnsafeBytes { raw in raw.count }
    total += empty.withUnsafeBytes { raw in raw.count + 100 }
    let doubled = text.withUnsafeBytes { raw -> Int in
        let n = raw.count
        return n * 2
    }
    return Int32(total + doubled)
}
