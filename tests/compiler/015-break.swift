// break leaves the loop at once.
func main() -> Int32 {
    var total: Int32 = 0
    for i in 0..<100 {
        if i == 4 { break }
        total = total + 1
    }
    return total
}
