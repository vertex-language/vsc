// vil: outer
func inner(_ n: Int) -> Int { return n }
func outer(_ n: Int) -> Int { return inner(n) }
