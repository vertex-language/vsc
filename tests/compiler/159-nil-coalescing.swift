// `??` is the conditional operator's shape asked of the case rather
// than of a bit: switch_enum says which case the optional holds, the
// some arm hands its payload to the join, and the none arm evaluates
// the right operand. Only the arm that runs evaluates, which is what
// the @autoclosure on Swift's right operand promises.
func pick(_ a: Int32?) -> Int32 {
    return a ?? 7
}

func widen(_ a: Int?) -> Int {
    return a ?? 100
}

// The fallback is an expression, not a constant, and is evaluated
// only where it is needed.
func costly() -> Int32 {
    var n: Int32 = 0
    var i: Int32 = 0
    while i < 5 {
        n = n + i
        i = i + 1
    }
    return n
}

func orCostly(_ a: Int32?) -> Int32 {
    return a ?? costly()
}

func main() -> Int32 {
    let some: Int32? = 5
    let none: Int32? = nil
    let total = pick(some) + pick(none) + orCostly(some) + orCostly(none)
    return total + Int32(widen(nil) - 100)
}
