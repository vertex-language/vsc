// A catch binds the error, an inner do catches before an outer one,
// and a throw out of a catch body reaches the enclosing handler.
struct Bad: Error {
    var code: Int
}

final class Log {
    var n = 0
    func add(_ d: Int) { n = (n * 10 + d) % 1000003 }
}

func fail(_ log: Log, _ x: Int) throws {
    defer { log.add(1) }
    if x != 0 {
        throw Bad(code: x)
    }
    log.add(2)
}

func relay(_ log: Log, _ x: Int) throws {
    do {
        try fail(log, x)
    } catch let e {
        _ = e
        log.add(3)
        throw Bad(code: 9)
    }
    log.add(4)
}

func main() -> Int32 {
    let log = Log()
    do {
        do {
            try fail(log, 1)
            log.add(9)
        } catch {
            log.add(5)
        }
        try relay(log, 0)
        try relay(log, 2)
        log.add(9)
    } catch {
        log.add(6)
    }
    log.add(7)
    return Int32(log.n % 251)
}
