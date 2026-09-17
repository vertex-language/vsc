// defer runs when its scope ends, last one first, on every way out:
// falling off the end, an early return, break and continue.
final class Log {
    var n = 0
    func add(_ d: Int) { n = (n * 10 + d) % 1000003 }
}

func early(_ log: Log, _ x: Int) -> Int {
    defer { log.add(1) }
    defer { log.add(2) }
    if x > 0 {
        return x
    }
    log.add(3)
    return 0
}

func loops(_ log: Log) {
    for i in 0..<4 {
        defer { log.add(4) }
        if i == 1 { continue }
        if i == 3 { break }
        log.add(5)
    }
}

func nested(_ log: Log) {
    defer { log.add(6) }
    do {
        defer { log.add(7) }
        log.add(8)
    }
    log.add(9)
}

func main() -> Int32 {
    let log = Log()
    _ = early(log, 1)
    _ = early(log, 0)
    loops(log)
    nested(log)
    return Int32(log.n % 251)
}
