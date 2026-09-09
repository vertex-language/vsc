// Shifting left multiplies; shifting a signed value right keeps its sign.
func main() -> Int32 {
    let a: Int32 = 3
    let left = a << 4
    let b: Int32 = -16
    let right = b >> 2
    return left + right
}
