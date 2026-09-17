typealias Handler = (Int) -> Void
func register(_ h: Handler) { h(1) }
func use() { register({ n in _ = n }) }
