// A range in a case pattern. It matches everything between its
// bounds, so what has to agree with the subject is the range's
// element rather than the range -- comparing the range itself made
// every range pattern an error, at every subject type. And the
// bounds take the subject's type, so the literals do not default to
// Int and answer a range of the wrong element.
//
// Lowered as two comparisons rather than one equality, closed or
// half-open as the operator says.
func grade(_ n: Int32) -> Int32 {
    switch n {
    case 0..<10: return 1
    case 10...20: return 2
    case 21...30: return 3
    default: return 9
    }
}

// The same at the subject's default width, where the literals need no
// adopting at all.
func wide(_ n: Int) -> Int {
    switch n {
    case 1...5: return 2
    default: return 4
    }
}

func main() -> Int32 {
    let edges = grade(0) + grade(9) + grade(10) + grade(20) + grade(25) + grade(99)
    return edges + Int32(wide(3)) + Int32(wide(9))
}
