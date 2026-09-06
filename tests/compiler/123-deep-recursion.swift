// Twenty thousand frames.
func down(_ n: Int32) -> Int32 { if n == 0 { return 0 }; return 1 + down(n - 1) }
func main() -> Int32 { return down(20000) / 500 - 358 }
