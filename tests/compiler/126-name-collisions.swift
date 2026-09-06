// A function whose name looks like a type's.
struct Value { var n: Int32 }
func Valued() -> Int32 { return 42 }
func main() -> Int32 { let v = Value(n: 0); return Valued() + v.n }
