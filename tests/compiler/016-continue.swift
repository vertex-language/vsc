// continue skips the rest of one iteration.
func main() -> Int32 {
    var total: Int32 = 0
    for i in 0..<10 {
        if i % 2 == 0 { continue }
        total = total + 1
    }
    return total
}
