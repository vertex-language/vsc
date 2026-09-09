// Swift's shift answers for every count: past the width, and
// negative, which the machine's shift says nothing about.
func main() -> Int32 {
    let a: Int32 = 1
    let neg: Int32 = -256
    var n: Int32 = 0
    if a << 40 == 0 { n += 1 }
    if a << -2 == 0 { n += 2 }
    if neg >> 40 == -1 { n += 4 }
    if neg >> -2 == -1024 { n += 8 }
    if a >> 40 == 0 { n += 16 }
    if a << 3 == 8 { n += 32 }
    if neg >> 2 == -64 { n += 64 }
    return n
}
