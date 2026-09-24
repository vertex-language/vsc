// Requirements that are async, throwing, mutating and take inout
// buffers, met by a struct and by a class, and called both through a
// generic parameter and through an existential: the value, the
// mutation and the error all reach the caller.

enum Bad: Error { case broken(Int) }

protocol Reader {
    mutating func read(into buffer: inout [UInt8]) throws -> Int
}

protocol AsyncReader {
    mutating func read(into buffer: inout [UInt8]) async throws -> Int
}

struct Bytes: Reader {
    var left: Int
    mutating func read(into buffer: inout [UInt8]) throws -> Int {
        if left == 0 { throw Bad.broken(0) }
        left -= 1
        buffer[0] = 7
        return 1
    }
}

final class Slow: AsyncReader {
    var calls = 0
    func read(into buffer: inout [UInt8]) async throws -> Int {
        calls += 1
        if calls > 2 { throw Bad.broken(calls) }
        buffer[0] = 9
        return 1
    }
}

func drain<R: AsyncReader>(_ r: inout R) async throws -> Int {
    var buf = [UInt8](repeating: 0, count: 4)
    var total = 0
    while true {
        total += try await r.read(into: &buf)
    }
}

func main() async -> Int32 {
    var status: Int32 = 0
    var buf = [UInt8](repeating: 0, count: 4)

    var e: any Reader = Bytes(left: 1)
    do {
        let n = try e.read(into: &buf)
        if n == 1 && buf[0] == 7 { status += 1 }
        _ = try e.read(into: &buf)
        status += 100
    } catch {
        status += 2
    }

    var s = Slow()
    do {
        _ = try await drain(&s)
        status += 100
    } catch Bad.broken(let n) {
        if n == 3 { status += 4 }
    } catch {
        status += 100
    }

    var a: any AsyncReader = Slow()
    do {
        let n = try await a.read(into: &buf)
        if n == 1 && buf[0] == 9 { status += 8 }
        _ = try await a.read(into: &buf)
        _ = try await a.read(into: &buf)
        status += 100
    } catch {
        status += 16
    }
    let none = try? await a.read(into: &buf)
    if none == nil { status += 32 }
    return status
}
