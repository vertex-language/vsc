// A requirement met by a method that does less -- synchronous for an
// async requirement, non-throwing for a throwing one -- and by the right
// one of two overloads of its name.

protocol Closer {
    mutating func close() throws
}

protocol AsyncWriter {
    mutating func write(_ bytes: [UInt8]) async throws
    mutating func flush() async throws
}

final class Pipe: Closer, AsyncWriter {
    var got: [UInt8] = []
    var closed = false
    var flushed = 0
    func close() { closed = true }
    func write(_ text: String) async throws {
        for b in text.utf8 { got.append(b) }
    }
    func write(_ bytes: [UInt8]) async throws {
        for b in bytes { got.append(b + 1) }
    }
    func flush() { flushed += 1 }
}

func shut<C: Closer>(_ c: inout C) throws {
    try c.close()
}

func main() async -> Int32 {
    var p = Pipe()
    var status: Int32 = 0
    do {
        var w: any AsyncWriter = p
        try await w.write([1, 2])
        try await w.flush()
        if p.got.count == 2 && p.got[0] == 2 && p.got[1] == 3 { status += 1 }
        if p.flushed == 1 { status += 2 }
        var c: any Closer = p
        try c.close()
        if p.closed { status += 4 }
        p.closed = false
        try shut(&p)
        if p.closed { status += 8 }
    } catch {
        status += 100
    }
    return status
}
