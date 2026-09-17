// A thrown error unwinds through callers that only `try`, running
// their defers, to the nearest catch.
struct Bad: Error {
    var code: Int
}

enum Failure: Error {
    case zero
    case big
}

final class Log {
    var n = 0
    func add(_ d: Int) { n = (n * 10 + d) % 1000003 }
}

func check(_ log: Log, _ x: Int) throws -> Int {
    defer { log.add(1) }
    if x < 0 { throw Bad(code: x) }
    if x == 0 { throw Failure.zero }
    if x > 100 { throw Failure.big }
    return x * 2
}

func twice(_ log: Log, _ x: Int) throws -> Int {
    defer { log.add(2) }
    let a = try check(log, x)
    return try check(log, a)
}

func run(_ log: Log, _ x: Int) -> Int {
    do {
        defer { log.add(3) }
        let v = try twice(log, x)
        log.add(4)
        return v
    } catch {
        log.add(5)
    }
    return -1
}

func main() -> Int32 {
    let log = Log()
    var total = 0
    total += run(log, 3)
    total += run(log, -1)
    total += run(log, 0)
    total += run(log, 60)
    return Int32((log.n + total) % 251)
}
