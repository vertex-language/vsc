// A binding in a case pattern is what the where clause is written
// about, so it has to be declared before the condition is read. It
// was read first, and reported the name it was about to declare.
func classify(_ n: Int32) -> Int32 {
    switch n {
    case let k where k < 0: return -k
    case let k where k > 100: return 100
    case 0...10: return 1
    default: return 0
    }
}
