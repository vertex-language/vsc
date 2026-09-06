// One expression, nested deeply enough to need the stack.
func main() -> Int32 {
  let a: Int32 = 1
  return ((((a + 1) * 2 + 3) * 2 - 4) * 2 + 5) * 2 - 4
}
