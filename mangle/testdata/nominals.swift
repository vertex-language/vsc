public func g0(alpha a: Int, _ b: Int) {}
public func g1(_ a: Int, beta b: Int) {}
public func g2(_ a: Int, _ b: Int, _ c: Int) {}
public func g3(_ a: Int, _ b: Bool, _ c: Int) {}
public func g4() -> (Int, Int) { return (0,0) }
public func g5(_ a: [Int]) {}
public func g6(_ a: Int?) {}
public struct P { public var x: Int; public init(x: Int) { self.x = x } }
public func g7(_ p: P) -> P { return p }
public class C { public func m(_ a: Int) -> Int { return a } }
public func g8(_ f: (Int) -> Int) {}
public func g9(_ a: inout Int) {}
public func g10() throws {}
