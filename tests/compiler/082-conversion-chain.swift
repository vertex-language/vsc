// A value through every width and back.
func main() -> Int32 {
    let a: Int8 = 100
    let b = Int16(a)
    let c = Int32(b)
    let d = Int64(c)
    let e = Int(d)
    let f = Int32(e)
    let g = Int16(f)
    let h = Int8(g)
    return Int32(h) - 58
}
