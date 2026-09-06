// The sign of a remainder follows the dividend.
func main() -> Int32 {
    let a: Int32 = -7 % 3
    let b: Int32 = 7 % -3
    let c: Int32 = -7 / 3
    return (a + 10) * 100 + (b + 10) * 10 + (c + 10)
}
