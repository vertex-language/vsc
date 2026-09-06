// Loops inside switches inside loops.
func main() -> Int32 {
    var total: Int32 = 0
    for i in 0..<6 {
        switch i % 3 {
        case 0:
            for j in 0..<3 {
                if j == 1 { continue }
                total = total + 1
            }
        case 1:
            var k: Int32 = 0
            while k < 3 {
                if k == 2 { break }
                total = total + 10
                k = k + 1
            }
        default:
            total = total + 100
        }
    }
    return total
}
