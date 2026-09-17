// async functions await each other, wait on the executor through
// Task.yield and Task.sleep, fail with try await, and are passed as
// closures. A task started with Task runs beside main; the program's
// status depends only on what main itself awaits, since Swift runs tasks
// on threads of its own and the order they interleave in is not fixed.
struct Bad: Error {
    var code: Int
}

enum Failure: Error {
    case late(Int)
}

func square(_ n: Int) async -> Int {
    await Task.yield()
    return n * n
}

func sumOfSquares(_ n: Int) async -> Int {
    var total = 0
    for i in 1...n {
        total += await square(i)
    }
    return total
}

func checked(_ n: Int) async throws -> Int {
    try await Task.sleep(nanoseconds: 1_000_000)
    if n < 0 {
        throw Bad(code: n)
    }
    if n > 100 {
        throw Failure.late(n)
    }
    return await square(n) + 1
}

func relay(_ n: Int) async throws -> Int {
    do {
        return try await checked(n)
    } catch let b as Bad {
        throw Bad(code: b.code * 2)
    }
}

func twice(_ n: Int, _ f: (Int) async -> Int) async -> Int {
    return await f(await f(n))
}

func background() async {
    for _ in 0..<3 {
        await Task.yield()
    }
}

func main() async -> Int32 {
    Task {
        await background()
    }
    var total = await sumOfSquares(4)
    total += await twice(3) { n in
        await Task.yield()
        return n + 10
    }
    do {
        total += try await relay(5)
        total += try await relay(-3)
        total += 1000
    } catch let b as Bad {
        total += -b.code * 7
    } catch {
        total += 2000
    }
    do {
        _ = try await relay(500)
        total += 3000
    } catch Failure.late(let n) {
        total += n / 100
    } catch {
        total += 4000
    }
    if let v = try? await checked(2) {
        total += v
    }
    try? await Task.sleep(nanoseconds: 2_000_000)
    return Int32(total % 251)
}
