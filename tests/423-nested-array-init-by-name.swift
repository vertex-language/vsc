// A nested array type and an array of optionals initialized by name.
struct Level { let w: Int }
struct Binding { let slot: Int }
let n = 3
var levels = [[Level]](repeating: [], count: n)
levels[1].append(Level(w: 4))
print(levels.count, levels[1][0].w, levels[0].isEmpty)
var bindings = [Binding?](repeating: nil, count: n)
bindings[2] = Binding(slot: 5)
print(bindings.map { $0?.slot ?? -1 })
let grid = [[Int]](repeating: [0, 0], count: 2)
print(grid)
