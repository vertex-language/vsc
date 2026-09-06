// A label says which loop to leave.
func main() -> Int32 {
    var total: Int32 = 0
    outer: for i in 0..<5 {
        for j in 0..<5 {
            if j == 3 { continue outer }
            if i == 4 { break outer }
            total = total + 1
        }
    }
    return total
}
