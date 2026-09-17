// A call that may fail.
//
// Swift carries the failure beside the result: the caller clears the
// error register before the call, the callee writes it on the path
// that fails, and the caller reads it after. swiftc's own code is
// `mov x21, #0` before the branch and `cbnz x21` after it, and a
// function that returns normally leaves the register as it found it.
//
// So one call has two edges out of it, which is what `try_apply`
// says. What this compiler does with the error is nothing: `try?`
// discards it and produces an optional, and that needs no error value
// at all. Anything that looks at the error needs `any Error`, whose
// conformance this compiler still names its own way.
public struct Bad: Error {
    public var code: Int32
    public init(code: Int32) { self.code = code }
}

public func mustBePositive(_ n: Int32) throws -> Int32 {
    if n < 0 { throw Bad(code: n) }
    return n
}

public func halved(_ n: Int32) throws -> Int32 {
    if n % 2 != 0 { throw Bad(code: n) }
    return n / 2
}

// A throwing call with an ordinary argument after it, where clearing
// the wrong register would show as a shifted argument list.
public func between(_ n: Int32, _ low: Int32, _ high: Int32) throws -> Int32 {
    if n < low || n > high { throw Bad(code: n) }
    return n
}

// One that never fails, which is the case a caller that forgot to
// clear the register gets wrong: the callee leaves it alone, so what
// the caller reads is whatever was there.
public func always(_ n: Int32) throws -> Int32 { return n }
