// A local name hides a function of the same name.
func value() -> Int32 { return 1 }
func main() -> Int32 { let value: Int32 = 42; return value }
