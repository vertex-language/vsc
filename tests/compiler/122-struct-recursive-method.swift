// A method that builds another instance and calls itself on it.
struct N { var v: Int32
  func down() -> Int32 { if v <= 0 { return 0 }; return v + N(v: v - 1).down() } }
func main() -> Int32 { return N(v: 8).down() + 6 }
