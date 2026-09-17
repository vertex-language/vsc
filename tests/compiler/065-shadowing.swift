// An inner binding hides an outer one for its scope only.
func main() -> Int32 {
    let n: Int32 = 1
    var total: Int32 = 0
    if true {
        let n: Int32 = 10
        total = total + n
    }
    total = total + n
    for _ in 0..<1 {
        let n: Int32 = 100
        total = total + n
    }
    return total + n
}
