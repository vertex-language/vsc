// A loop tested at the bottom always runs once.
func main() -> Int32 {
    var n: Int32 = 100
    var runs: Int32 = 0
    repeat {
        runs = runs + 1
        n = n + 1
    } while n < 5
    return runs
}
