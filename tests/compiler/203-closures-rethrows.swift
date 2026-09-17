// A closure may throw: written in parentheses or trailing, it takes a
// throwing type from the parameter it is passed to. A rethrows function
// fails only when the closure it is given does. A function declared
// inside a throwing one does not disturb where the outer one's errors go.
struct Bad: Error {
    var code: Int
}

final class Log {
    var n = 0
    func add(_ d: Int) { n = (n * 10 + d) % 1000003 }
}

func apply(_ x: Int, _ f: (Int) throws -> Int) throws -> Int {
    return try f(x) + 1
}

func mapAll(_ xs: [Int], _ f: (Int) throws -> Int) rethrows -> Int {
    var total = 0
    for x in xs {
        total += try f(x)
    }
    return total
}

func checked(_ log: Log, _ x: Int) throws -> Int {
    func double(_ y: Int) -> Int {
        return y * 2
    }
    do {
        if x < 0 {
            throw Bad(code: x)
        }
        return double(x)
    } catch let b as Bad {
        log.add(1)
        throw Bad(code: b.code - 1)
    }
}

func main() -> Int32 {
    let log = Log()
    var total = 0
    do {
        total += try apply(3, { x in x * 2 })
        total += try apply(4) { $0 + 10 }
        total += try apply(-1) { x in
            if x < 0 {
                throw Bad(code: x)
            }
            return x
        }
        log.add(9)
    } catch let b as Bad {
        log.add(2)
        total += b.code * -5
    } catch {
        log.add(9)
    }

    total += mapAll([1, 2, 3]) { $0 * 10 }
    do {
        total += try mapAll([1, -2, 3]) { x in
            if x < 0 {
                throw Bad(code: x)
            }
            return x
        }
        log.add(9)
    } catch let b as Bad {
        log.add(3)
        total += b.code * -7
    } catch {
        log.add(9)
    }

    do {
        total += try checked(log, 6)
        total += try checked(log, -3)
        log.add(9)
    } catch let b as Bad {
        log.add(4)
        total += b.code * -11
    } catch {
        log.add(9)
    }
    return Int32((log.n + total * 7) % 251)
}
