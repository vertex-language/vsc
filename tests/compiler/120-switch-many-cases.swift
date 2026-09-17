// Ten cases and a default.
func v(_ n: Int32) -> Int32 {
  switch n {
  case 0: return 0
  case 1: return 1
  case 2: return 2
  case 3: return 3
  case 4: return 4
  case 5: return 5
  case 6: return 6
  case 7: return 7
  case 8: return 8
  case 9: return 9
  default: return 100
  }
}
func main() -> Int32 { return v(7) * 6 + v(50) / 100 * 0 + 0 }
