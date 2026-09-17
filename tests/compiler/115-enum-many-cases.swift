// Thirty cases, so the tag is wider than the switch's first few.
enum E {
  case c0, c1, c2, c3, c4, c5, c6, c7, c8, c9
  case d0, d1, d2, d3, d4, d5, d6, d7, d8, d9
  case e0, e1, e2, e3, e4, e5, e6, e7, e8, e9
}
func v(_ x: E) -> Int32 { switch x { case .e9: return 42; default: return 1 } }
func main() -> Int32 { return v(.e9) }
