// An async pipeline: stages as async functions, fan-out in a group, results in an actor.
actor Results {
    private var items: [Int: String] = [:]
    func put(_ k: Int, _ v: String) { items[k] = v }
    func all() -> [String] { items.keys.sorted().map { items[$0]! } }
}
func fetch(_ id: Int) async -> Int {
    await Task.yield()
    return id * 3
}
func render(_ id: Int, _ v: Int) async -> String { "\(id):\(v)" }
let results = Results()
await withTaskGroup(of: Void.self) { group in
    for id in 1...6 {
        group.addTask {
            let v = await fetch(id)
            await results.put(id, await render(id, v))
        }
    }
}
async let head = fetch(100)
print(await results.all(), await head)
