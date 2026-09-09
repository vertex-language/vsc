// Every width, so the bound the comparison uses is the type's own
// and not the register's.
func main() -> Int32 {
    var n: Int32 = 0
    let a: Int8 = 1
    let b: Int16 = 1
    let c: Int64 = 1
    let d: UInt8 = 0xF0
    if a << 9 == 0 { n += 1 }
    if b << 20 == 0 { n += 2 }
    if c << 70 == 0 { n += 4 }
    if d >> 9 == 0 { n += 8 }
    if a << 3 == 8 { n += 16 }
    if c << 40 == 1099511627776 { n += 32 }
    return n
}
