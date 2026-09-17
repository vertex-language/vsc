// A loop inside a loop runs the product of their counts.
func main() -> Int32 {
    var total: Int32 = 0
    for _ in 0..<4 {
        for _ in 0..<5 {
            total = total + 1
        }
    }
    return total
}
