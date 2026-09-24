// An optional's cases are .some and .none; `let x?` is only the first.
func pick(_ o: Int?) -> Int {
    switch o {
    case let x?: return x
    }
}
