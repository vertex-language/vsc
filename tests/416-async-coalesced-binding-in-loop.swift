// In an async function, inside a for loop: an if let over `??` of a
// first(where:) and a ternary, and a guard let that continues.
struct Disk { let name: String; let boot: Bool }
func tick() async -> Int { 1 }
func open(_ groups: [[Disk]], fallback: Bool) async -> [String] {
    var out: [String] = []
    for disks in groups {
        _ = await tick()
        if let p = disks.first(where: { $0.boot }) ?? (fallback ? disks.first : nil) {
            out.append(p.name)
        }
        guard let last = disks.last else { out.append("empty"); continue }
        out.append(last.name)
    }
    return out
}
let groups = [[Disk(name: "a", boot: false), Disk(name: "b", boot: true)], [Disk(name: "c", boot: false)], []]
print(await open(groups, fallback: true))
print(await open(groups, fallback: false))
