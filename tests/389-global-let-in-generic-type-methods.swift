// Module-level lets, a scalar and a table, read inside a generic type's
// methods.
let Size = 4
let neighbors: [(Int, Int)] = [(1, 0), (-1, 0), (0, 1), (0, -1)]
struct Chunk<T> {
    var cells: [T]
    init(fill: T) { cells = Array(repeating: fill, count: Size * Size) }
    func get(_ x: Int, _ y: Int) -> T { cells[y * Size + x] }
    func around(_ x: Int, _ y: Int) -> [T] {
        var out: [T] = []
        for (dx, dy) in neighbors {
            let nx = x + dx, ny = y + dy
            if nx >= 0 && nx < Size && ny >= 0 && ny < Size { out.append(get(nx, ny)) }
        }
        return out
    }
}
var c = Chunk(fill: 0)
c.cells[5] = 9
print(c.get(1, 1), c.around(1, 0), c.around(0, 0).count)
let s = Chunk(fill: "a")
print(s.around(3, 3))
