// Errors through methods, rethrown from a catch, and carried by enum
// cases whose associated values are labelled.
struct Bad: Error {
    var code: Int
}

enum Failure: Error {
    case limit(max: Int)
    case range(lo: Int, hi: Int)
}

final class Log {
    var n = 0
    func add(_ d: Int) { n = (n * 10 + d) % 1000003 }
}

struct Parser {
    var limit: Int
    func parse(_ x: Int) throws -> Int {
        if x > limit { throw Failure.limit(max: limit) }
        if x < 0 { throw Failure.range(lo: x, hi: limit) }
        return x * 2
    }
}

final class Conn {
    var open = true
    func send(_ n: Int) throws -> Int {
        if !open { throw Bad(code: n) }
        return n + 1
    }
}

func relay(_ log: Log, _ c: Conn, _ n: Int) throws -> Int {
    do {
        return try c.send(n)
    } catch {
        log.add(1)
        throw error
    }
}

func main() -> Int32 {
    let log = Log()
    var total = 0
    let p = Parser(limit: 10)
    do {
        total += try p.parse(3)
        total += try p.parse(30)
        log.add(9)
    } catch Failure.limit(let m) {
        log.add(2)
        total += m
    } catch {
        log.add(9)
    }
    do {
        _ = try p.parse(-4)
    } catch Failure.range(let lo, let hi) {
        log.add(3)
        total += hi - lo
    } catch {
        log.add(9)
    }
    if let v = try? p.parse(4) {
        total += v
    }
    if (try? p.parse(40)) == nil {
        log.add(4)
    }
    total += try! p.parse(5)

    let c = Conn()
    do {
        total += try relay(log, c, 1)
        c.open = false
        total += try relay(log, c, 2)
        log.add(9)
    } catch let b as Bad {
        log.add(5)
        total += b.code * 100
    } catch {
        log.add(9)
    }
    return Int32((log.n + total * 7) % 251)
}
