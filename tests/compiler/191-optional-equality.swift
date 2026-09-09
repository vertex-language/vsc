// Two optionals compared: equal payloads, different ones, and nil.
func main() -> Int32 {
    let a: Int32? = 7
    let b: Int32? = 7
    let c: Int32? = 9
    let z: Int32? = nil
    let y: Int32? = nil
    var n: Int32 = 0
    if a == b { n += 1 }
    if a != c { n += 2 }
    if z == y { n += 4 }
    if a != z { n += 8 }
    if !(z == a) { n += 16 }
    return n
}
