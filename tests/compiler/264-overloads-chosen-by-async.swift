// A synchronous function and an async one may share a name. In an async
// function, or a closure that awaits, the call is the async one; in a
// synchronous function it is the other. Methods are chosen the same way.
// This is how a package offers a blocking and an async form of one
// operation under one name.
func load(_ n: Int) -> Int { return n }
func load(_ n: Int) async -> Int {
    await Task.yield()
    return n * 10
}

struct Store {
    var base: Int
    func read() -> Int { return base }
    func read() async -> Int {
        await Task.yield()
        return base + 1000
    }
}

func fromSync() -> Int {
    return load(1) + Store(base: 5).read()
}

func fromAsync() async -> Int {
    return await load(2) + (await Store(base: 5).read())
}

func main() async -> Int32 {
    var failures: Int32 = 0
    if fromSync() != 6 { failures += 1 }
    if await fromAsync() != 1025 { failures += 1 }

    // A closure that awaits is async, and chooses as an async function does.
    let later = { () async -> Int in
        return await load(3)
    }
    if await later() != 30 { failures += 1 }

    // A closure that does not await is synchronous.
    let now = { () -> Int in
        return load(4)
    }
    if now() != 4 { failures += 1 }

    print(failures == 0 ? "ok" : "failed")
    return failures
}
