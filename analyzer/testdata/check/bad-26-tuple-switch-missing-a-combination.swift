// A tuple's values are every combination of its elements': (true, true)
// and (false, false) leave (true, false) and (false, true) out.
func both(_ t: (Bool, Bool)) -> Int {
    switch t {
    case (true, true): return 1
    case (false, false): return 2
    }
}
