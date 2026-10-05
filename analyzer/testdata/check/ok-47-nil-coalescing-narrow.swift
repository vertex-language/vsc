// `??` against an optional whose wrapped type is narrower than a
// literal's default. The right operand adopts the wrapped type rather
// than staying an Int, which is what swiftc does and what this
// compiler once got wrong: the case fell through and the result came
// out optional.
func pick(_ a: Int32?) -> Int32 { return a ?? 0 }

func widen(_ a: Int?) -> Int { return a ?? 0 }

// Swift's other overload: T? ?? T? is a T?.
func either(_ a: Int32?, _ b: Int32?) -> Int32? { return a ?? b }
