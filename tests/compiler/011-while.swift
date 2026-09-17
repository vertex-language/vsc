// A loop tested at the top.
func main() -> Int32 {
    var n: Int32 = 0
    var total: Int32 = 0
    while n < 5 {
        total = total + n
        n = n + 1
    }
    return total
}
