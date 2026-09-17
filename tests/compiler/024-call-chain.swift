// Functions calling functions calling functions.
func a(_ n: Int32) -> Int32 { return b(n) + 1 }
func b(_ n: Int32) -> Int32 { return c(n) * 2 }
func c(_ n: Int32) -> Int32 { return n - 3 }

func main() -> Int32 {
    return a(10)
}
