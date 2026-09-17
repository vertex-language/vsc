// More arguments than AArch64 has registers for them.
func ten(_ a: Int32, _ b: Int32, _ c: Int32, _ d: Int32, _ e: Int32,
         _ f: Int32, _ g: Int32, _ h: Int32, _ i: Int32, _ j: Int32) -> Int32 {
    return a + b + c + d + e + f + g + h + i + j
}

func main() -> Int32 {
    return ten(1, 2, 3, 4, 5, 6, 7, 8, 9, 3)
}
