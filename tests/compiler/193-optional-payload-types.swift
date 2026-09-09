// The comparison of the payloads is the type's own.
func main() -> Int32 {
    var n: Int32 = 0
    let a: UInt8? = 200
    let b: UInt8? = 200
    if a == b { n += 1 }
    let c: Int64? = -5
    if c == -5 { n += 2 }
    let e: Double? = 1.5
    if e == 1.5 { n += 4 }
    let f: Int8? = -128
    if f != 0 { n += 8 }
    return n
}
