// @MainActor, as Swift has it: a type whose members run on the main
// thread, functions marked so; nonisolated members read from anywhere;
// synchronous isolated code awaited from a detached task, which hops
// there and back; async isolated functions that get themselves there;
// MainActor.run and assumeIsolated; Task inheriting the main actor where
// Task.detached does not. Each isolated body checks where it is, so a
// hop that went missing ends the program.

// assumeIsolated is for synchronous code the compiler cannot see is on
// the main actor; from an async function Swift asks for an await instead.
@MainActor func here() {
    MainActor.assumeIsolated { }
}

@MainActor
final class Model {
    var count = 0
    let name: String
    init(name: String) { self.name = name }
    func bump() {
        MainActor.assumeIsolated { count += 1 }
    }
    nonisolated func label() -> String { return "model " + name }
    func load() async -> Int {
        here()
        await Task.yield()
        here()
        return count * 10
    }
}

@MainActor final class Screen {
    static var redraws = 0
}

@MainActor func redraw(_ n: Int) {
    MainActor.assumeIsolated { Screen.redraws += 1 }
    print("redraw", n)
}

func compute(_ m: Model) async -> Int {
    print(m.label())
    await m.bump()
    let c = await m.count
    await redraw(c)
    let l = await m.load()
    await MainActor.run {
        here()
        m.bump()
        print("ran on main", m.count)
    }
    await Task.yield()
    return l + c + (await Screen.redraws)
}

@MainActor func main() async -> Int32 {
    let m = Model(name: "one")
    m.bump()
    redraw(m.count)
    let detached = Task.detached { () -> Int in
        return await compute(m)
    }
    let r = await detached.value
    let inherited = Task { () -> Int in
        here()
        m.bump()
        return m.count
    }
    let i = await inherited.value
    print("result", r, "inherited", i, "count", m.count, "redraws", Screen.redraws)
    return 0
}
