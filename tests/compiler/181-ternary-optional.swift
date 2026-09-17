// A ternary arm injects into the optional the whole expression
// is, and break and continue leave a while let.
func main() -> Int32 {
    var n: Int32? = 10
    var total: Int32 = 0
    while let v = n {
        n = v == 1 ? nil : v - 1
        if v % 2 == 0 { continue }
        if v == 3 { break }
        total += v
    }
    return total
}
