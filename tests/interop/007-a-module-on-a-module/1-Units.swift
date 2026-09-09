// The lower of two libraries. Top imports this one, and the program
// imports Top -- so this module is reached through another module's
// interface rather than through anything the program wrote.
public struct Metric {
    public var raw: Int32
    public init(raw: Int32) { self.raw = raw }
    public func doubled() -> Int32 { return raw * 2 }
}

public func rawOf(_ n: Int32) -> Int32 { return n }
