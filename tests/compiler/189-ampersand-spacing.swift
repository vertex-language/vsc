// `&` followed by a sign is still `&` when the spacing says so.
func take(_ x: inout Int32) { x += 1 }
func main() -> Int32 {
    let a: Int32 = 0b1100
    var n: Int32 = 0
    if a & -1 == 12 { n += 1 }
    if a & +8 == 8 { n += 2 }
    if a &- 1 == 11 { n += 4 }
    var v: Int32 = 5
    take(&v)
    if v == 6 { n += 8 }
    return n
}
