// A function value applied to its own result.
func twice(_ f: (Int32) -> Int32, _ x: Int32) -> Int32 { return f(f(x)) }
func main() -> Int32 {
  let add: (Int32) -> Int32 = { $0 + 10 }
  return twice(add, 2) + twice({ $0 * 2 }, 5)
}
