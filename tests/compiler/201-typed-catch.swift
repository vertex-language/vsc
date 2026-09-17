// A catch clause can test the error: `let e as T` binds it as a T,
// `E.case(let x)` matches one case and binds what it carries, and `is T`
// tests the type alone. An error no clause matches goes on to the
// enclosing handler.
struct Bad: Error {
    var code: Int
}

enum Failure: Error {
    case zero
    case big(Int)
    case pair(Int, Int)
}

final class Log {
    var n = 0
    func add(_ d: Int) { n = (n * 10 + d) % 1000003 }
}

func check(_ x: Int) throws -> Int {
    if x < 0 { throw Bad(code: x) }
    if x == 0 { throw Failure.zero }
    if x > 1000 { throw Failure.pair(x, x / 2) }
    if x > 100 { throw Failure.big(x) }
    return x
}

func classify(_ log: Log, _ x: Int) -> Int {
    do {
        return try check(x)
    } catch let b as Bad {
        log.add(1)
        return -b.code
    } catch Failure.zero {
        log.add(2)
    } catch Failure.big(let n) {
        log.add(3)
        return n / 100
    } catch is Failure {
        log.add(4)
    } catch {
        log.add(5)
    }
    return 0
}

func partial(_ log: Log, _ x: Int) throws -> Int {
    defer { log.add(6) }
    do {
        return try check(x)
    } catch Failure.pair(let a, let b) {
        return a - b
    }
}

func outer(_ log: Log, _ x: Int) -> Int {
    do {
        return try partial(log, x)
    } catch let b as Bad {
        log.add(7)
        return b.code
    } catch {
        log.add(8)
    }
    return 0
}

func main() -> Int32 {
    let log = Log()
    var total = 0
    total += classify(log, 5)
    total += classify(log, -3)
    total += classify(log, 0)
    total += classify(log, 500)
    total += classify(log, 5000)
    total += outer(log, 2000)
    total += outer(log, -4)
    total += outer(log, 300)
    total += outer(log, 9)
    return Int32((log.n + total * 7) % 251)
}
