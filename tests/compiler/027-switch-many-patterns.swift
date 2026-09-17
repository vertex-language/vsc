// One case may name several values.
func kind(_ n: Int32) -> Int32 {
    switch n {
    case 1, 3, 5, 7, 9: return 1
    case 0, 2, 4, 6, 8: return 2
    default: return 3
    }
}

func main() -> Int32 {
    return kind(3) * 100 + kind(4) * 10 + kind(99)
}
