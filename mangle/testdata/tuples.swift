public struct A {}
public struct B {}
public func k0() -> A { return A() }
public func k1() -> (A, A) { return (A(), A()) }
public func k2(_ x: A) -> (A, B, A, B) { return (x, B(), x, B()) }
