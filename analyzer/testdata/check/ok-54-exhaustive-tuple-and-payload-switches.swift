// Switches that cover every value of a tuple, of an enum whose cases carry
// values, and of optionals of both, by combinations of cases rather than
// one pattern that matches everything. A case with a where clause covers
// nothing, as it may not match.
enum Inner { case x, y }
enum E { case a(Int, Bool), b, c(Inner) }
indirect enum Tree { case leaf, node(Tree, Tree) }

func tuple(_ t: (Bool, Bool)) -> Int {
    switch t {
    case (true, _): return 1
    case (false, true): return 2
    case (_, false): return 3
    }
}

func payloads(_ e: E) -> Int {
    switch e {
    case .a(_, true): return 1
    case .a(let n, false): return n
    case .b: return 2
    case .c(.x): return 3
    case .c(.y): return 4
    }
}

func optionalEnum(_ o: E?) -> Int {
    switch o {
    case .a?: return 1
    case .b?: return 2
    case .c(let i)?: return payloads(.c(i))
    case nil: return 0
    }
}

func optionalInTuple(_ b: Bool?, _ n: Int) -> Int {
    switch (b, n) {
    case (true?, _): return 1
    case (false?, let k): return k
    case (nil, _): return 0
    }
}

func guarded(_ b: Bool, _ n: Int) -> Int {
    switch (b, n) {
    case (true, let k) where k > 0: return 1
    case (true, _): return 2
    case (false, _): return 3
    }
}

func tree(_ t: Tree) -> Int {
    switch t {
    case .leaf: return 0
    case .node(.leaf, _): return 1
    case .node(.node, _): return 2
    }
}

func whole(_ e: E) -> Int {
    switch e {
    case .a(let pair): return pair.0
    case .b, .c: return 0
    }
}
