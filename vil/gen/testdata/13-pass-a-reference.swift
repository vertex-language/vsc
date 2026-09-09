// vil: gives
final class Box { var n: Int = 0 }
func takes(_ b: Box) -> Int { return b.n }
func gives(_ b: Box) -> Int { return takes(b) }
