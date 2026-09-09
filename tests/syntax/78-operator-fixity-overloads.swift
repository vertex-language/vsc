// §6.9 Operator Fixity Overloading and Unicode Arrow Operators

prefix operator +++
postfix operator +++
infix operator +++: AdditionPrecedence

prefix operator ↔
infix operator ↔: LogicalConjunctionPrecedence
infix operator →: LogicalConjunctionPrecedence

struct Counter {
    var value: Int
}

prefix func +++(c: inout Counter) -> Counter {
    c.value += 2
    return c
}

postfix func +++(c: inout Counter) -> Counter {
    let old = c
    c.value += 2
    return old
}

func +++(lhs: Counter, rhs: Counter) -> Counter {
    return Counter(value: lhs.value + rhs.value)
}

func ↔(lhs: Bool, rhs: Bool) -> Bool {
    return lhs == rhs
}

prefix func ↔(val: Bool) -> Bool {
    return val
}

func →(lhs: Bool, rhs: Bool) -> Bool {
    return !lhs || rhs
}

func testFixityOverloads() {
    var c = Counter(value: 0)
    let pre = +++c
    let post = c+++
    let combined = pre +++ post
    let logic = (true ↔ false) → (false ↔ true)
    let pLogic = ↔true
    _ = (c, pre, post, combined, logic, pLogic)
}
