// `..<` stops before its upper bound.
func main() -> Int32 {
    var total: Int32 = 0
    for i in 0..<5 {
        total = total + Int32(i)
    }
    return total
}
