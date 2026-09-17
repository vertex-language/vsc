// Nine arms, taken in order.
func band(_ n: Int32) -> Int32 {
  if n < 1 { return 1 } else if n < 2 { return 2 } else if n < 3 { return 3 }
  else if n < 4 { return 4 } else if n < 5 { return 5 } else if n < 6 { return 6 }
  else if n < 7 { return 7 } else if n < 8 { return 8 } else { return 9 }
}
func main() -> Int32 { return band(6) * 6 }
