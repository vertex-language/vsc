// An Int8 accumulated in a loop, staying in range.
func main() -> Int32 {
  var t: Int8 = 0
  for _ in 0..<6 { t = t + 7 }
  return Int32(t)
}
