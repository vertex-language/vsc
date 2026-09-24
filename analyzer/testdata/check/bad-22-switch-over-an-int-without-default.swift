// A switch over an Int covers every value only with a pattern that
// matches any: the literals it names leave the rest.
func pick(_ n: Int) -> Int {
    switch n {
    case 1: return 1
    case 2: return 2
    }
}
