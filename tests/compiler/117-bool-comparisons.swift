// Bool compared with Bool.
func main() -> Int32 {
  let t = true; let f = false
  var s: Int32 = 0
  if t == true { s = s + 1 }
  if f != true { s = s + 2 }
  if (t && !f) == true { s = s + 4 }
  return s + 35
}
