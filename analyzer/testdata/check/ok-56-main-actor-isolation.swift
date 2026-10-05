// What @MainActor allows: its own code calling its own, synchronous or
// not; nonisolated members from anywhere; async isolated functions
// awaited from anywhere; synchronous ones awaited from async code; a
// closure made on the main thread calling into it; Task inheriting.
@MainActor
final class Model {
    var count = 0
    let name: String
    init(name: String) { self.name = name }
    func bump() { count += 1 }
    nonisolated func label() -> String { return "model" }
    func load() async -> Int {
        await Task.yield()
        return count
    }
}

@MainActor var total = 0
@MainActor func redraw(_ n: Int) { total += n }

func compute(_ m: Model) async -> Int {
    await m.bump()
    let c = await m.count
    await redraw(c)
    let l = await m.load()
    let t = await total
    print(m.label())
    return l + c + t
}

@MainActor func onMain(_ m: Model) {
    m.bump()
    redraw(m.count)
    total += 1
    let f = { m.bump() }
    f()
}

@MainActor func start() {
    let model = Model(name: "m")
    model.bump()
    redraw(model.count)
    Task { model.bump() }
    Task.detached { _ = await compute(model) }
    onMain(model)
}
