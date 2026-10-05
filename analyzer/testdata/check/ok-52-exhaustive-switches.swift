// Switches that cover every value without a default: a Bool by both
// literals, an optional by its two cases however they are spelled, and
// any type by a pattern that binds or ignores the value.
func bool(_ b: Bool) -> Int {
    switch b {
    case true: return 1
    case false: return 0
    }
}

func optional(_ o: Int?) -> Int {
    switch o {
    case let x?: return x
    case nil: return 0
    }
}

func spelled(_ o: Int?) -> Int {
    switch o {
    case .some(let x): return x
    case .none: return 0
    }
}

func any(_ n: Int) -> Int {
    switch n {
    case 1: return 1
    case let k: return k
    }
}

func ignored(_ s: String) -> Int {
    switch s {
    case "a": return 1
    case _: return 0
    }
}
