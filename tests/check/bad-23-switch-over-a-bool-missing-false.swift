// A Bool has two values, and a switch has to name both or catch the rest.
func pick(_ b: Bool) -> Int {
    switch b {
    case true: return 1
    }
}
