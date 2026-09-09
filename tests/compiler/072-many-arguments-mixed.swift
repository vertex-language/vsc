// Past the register count, with widths that differ.
func mixed(_ a: Int8, _ b: Int16, _ c: Int32, _ d: Int64,
           _ e: Int8, _ f: Int16, _ g: Int32, _ h: Int64,
           _ i: Int32, _ j: Int32) -> Int32 {
    return Int32(a) + Int32(b) + c + Int32(d) + Int32(e) + Int32(f) + g + Int32(h) + i + j
}

func main() -> Int32 {
    return mixed(1, 2, 3, 4, 5, 6, 7, 8, 9, 3)
}
