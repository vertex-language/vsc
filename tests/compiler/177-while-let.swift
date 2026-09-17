// while let rebinds on every pass.
func main() -> Int32 {
    var n: Int32? = 5
    var total: Int32 = 0
    while let v = n {
        total += v
        if v == 1 { n = nil } else { n = v - 1 }
    }
    return total
}
