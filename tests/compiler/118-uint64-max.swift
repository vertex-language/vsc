// The whole unsigned range, divided and remaindered.
func opaque(_ n: UInt64) -> UInt64 { return n }
func main() -> Int32 {
  let m = opaque(18446744073709551615)
  var s: Int32 = 0
  if m > 0 { s = s + 1 }
  if m / 2 == 9223372036854775807 { s = s + 2 }
  if m % 10 == 5 { s = s + 4 }
  return s + 35
}
