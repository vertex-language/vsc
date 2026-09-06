// A thousand frames deep and back.
func down(_ n: Int32) -> Int32 {
    if n == 0 { return 0 }
    return 1 + down(n - 1)
}

func main() -> Int32 {
    return down(1000) / 10
}
