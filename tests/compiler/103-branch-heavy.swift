// Enough branches to make the block order matter.
func f(_ n: Int32) -> Int32 {
  var t: Int32 = 0
  if n > 0 { t = t + 1 } else { t = t - 1 }
  if n > 1 { t = t + 2 } else { t = t - 2 }
  if n > 2 { t = t + 4 } else { t = t - 4 }
  if n > 3 { t = t + 8 } else { t = t - 8 }
  if n > 4 { t = t + 16 } else { t = t - 16 }
  return t
}
func main() -> Int32 { return f(10) + 11 }
