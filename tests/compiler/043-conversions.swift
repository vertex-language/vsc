// A conversion checks that the value fits, then changes width.
func main() -> Int32 {
    let a: Int32 = 40
    let wide = Int(a)
    let back = Int32(wide)
    let small = Int8(back / 20)
    return back + Int32(small)
}
