// A tuple pattern matches element by element, so each element is
// checked against the subject's element at that position. Passing the
// whole tuple down made every element an error about a scalar that
// cannot match a tuple, and gave a binding the tuple's type rather
// than its own.
func pairKind(_ p: (Int32, Int32)) -> Int32 {
    switch p {
    case (0, 0): return 1
    case (let a, 0): return a
    case (0, let b): return b
    case (let a, let b): return a + b
    }
}
