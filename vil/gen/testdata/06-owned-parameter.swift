// vil: consumes
final class Box { var n: Int = 0 }
func consumes(_ b: __owned Box) -> Box { return b }
