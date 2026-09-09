// A wider accumulator than the loop's own index.
func main() -> Int32 {
  var total: Int64 = 0
  for i in 0..<1000 { total = total + Int64(i) }
  return Int32(total / 11893)
}
