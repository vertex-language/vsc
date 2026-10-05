// A task group runs a dynamic number of children; results arrive in any order.
let total = await withTaskGroup(of: (Int, Int).self) { group in
    for i in 1...10 {
        group.addTask { (i, i * i) }
    }
    var pairs: [(Int, Int)] = []
    for await p in group { pairs.append(p) }
    return pairs.sorted { $0.0 < $1.0 }
}
print(total.map(\.1), total.map(\.1).reduce(0, +))
