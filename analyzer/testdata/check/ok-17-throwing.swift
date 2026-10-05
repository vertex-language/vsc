enum Failure: Error {
    case broken
}

func mayThrow() throws {}
func typedThrow() throws(Failure) {}
func neverThrows() throws(Never) {}

func use() throws {
    try mayThrow()
    try typedThrow()
    neverThrows()
}
