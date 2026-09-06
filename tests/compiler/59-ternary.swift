// The conditional expression picks one side and evaluates only it.
func main() -> Int32 {
    let a: Int32 = 5
    let b: Int32 = 9
    let big = a > b ? a : b
    let small = a < b ? a : b
    return big * 10 + small
}
