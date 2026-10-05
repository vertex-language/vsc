// Two things whose ABI is not the ordinary one, and which agree with
// swiftc only if this compiler says what swiftc says.
//
// A class's method takes its receiver in the self register, x20,
// rather than in the argument sequence -- while a value type's method
// takes it as its last ordinary argument. A result too wide for
// registers is not returned at all: the caller sets storage aside and
// passes its address in x8, beside the sequence rather than in it.
//
// Neither is visible in a signature or a symbol. Both are the kind of
// disagreement that links and runs and answers.
public final class Counter {
    public var n: Int32
    public init(n: Int32) { self.n = n }
    public func plus(_ k: Int32) -> Int32 { return n + k }
    public func scaled(by k: Int32) -> Int32 { return n * k }
}

public func makeCounter(_ n: Int32) -> Counter { return Counter(n: n) }

public struct Big {
    public var a: Int
    public var b: Int
    public var c: Int
    public var d: Int
    public var e: Int
    public init(a: Int, b: Int, c: Int, d: Int, e: Int) {
        self.a = a; self.b = b; self.c = c; self.d = d; self.e = e
    }
}

public func makeBig(_ n: Int) -> Big { return Big(a: n, b: n + 1, c: n + 2, d: n + 3, e: n + 4) }
public func sumBig(_ w: Big) -> Int { return w.a + w.b + w.c + w.d + w.e }
