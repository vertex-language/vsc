// Multiplication binds tighter than addition, and parentheses win.
func main() -> Int32 {
    let a: Int32 = 2 + 3 * 4
    let b: Int32 = (2 + 3) * 4
    return b - a
}
