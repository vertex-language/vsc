// Branches inside branches inside a loop.
func main() -> Int32 {
  var t: Int32 = 0
  for i in 0..<20 {
    if i % 2 == 0 { if i % 4 == 0 { t = t + 3 } else { t = t + 1 } }
    else { if i % 3 == 0 { t = t + 2 } else { t = t - 1 } }
  }
  return t + 33
}
