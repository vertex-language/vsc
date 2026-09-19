// Subscripts a type declares: read through the getter and written
// through the setter, with labels or without, static, on a class, in an
// extension, through `self`, and written through -- `m[1][0] = 7`,
// `b[at: 0].n += 10`, `m[1].append(8)` -- by way of a temporary set back
// once the statement is done. Nested tuple parameters.

struct Grid {
    var cells: [Int]
    let width: Int
    subscript(x: Int, y: Int) -> Int {
        get { return cells[y * width + x] }
        set { cells[y * width + x] = newValue }
    }
    subscript(i: Int) -> Int { return cells[i] }
    subscript(name: String) -> String { return "\(name):\(width)" }
}
final class Registry {
    var items: [String: Int] = [:]
    subscript(key: String) -> Int {
        get { return items[key] ?? 0 }
        set { items[key] = newValue }
    }
    static subscript(n: Int) -> Int { return n * 2 }
}
struct Matrix {
    var rows: [[Int]]
    subscript(r: Int) -> [Int] {
        get { return rows[r] }
        set { rows[r] = newValue }
    }
}
struct Cell { var n: Int; var tag: String }
struct Board {
    var cells: [Cell] = [Cell(n: 1, tag: "a"), Cell(n: 2, tag: "b")]
    subscript(at i: Int) -> Cell {
        get { return cells[i] }
        set { cells[i] = newValue }
    }
    subscript(tag t: String) -> Int? {
        for c in cells where c.tag == t { return c.n }
        return nil
    }
    mutating func double(_ i: Int) { self[at: i].n *= 2 }
    func first() -> Cell { return self[at: 0] }
}
extension Board {
    subscript(sum a: Int, _ b: Int) -> Int { return self[at: a].n + self[at: b].n }
}
enum Suit: Int {
    case hearts = 1, spades = 2
    subscript(scale: Int) -> Int { return self.rawValue * scale }
}
final class Bag {
    var items: [String] = []
    subscript(i: Int) -> String {
        get { return items[i] }
        set { items[i] = newValue }
    }
    subscript(prefix p: String) -> [String] {
        var out: [String] = []
        for s in items where s.hasPrefix(p) { out.append(s) }
        return out
    }
}

func f(_ t: ((Int, Int), Int)) -> Int { return t.0.0 + t.0.1 + t.1 }
func g(_ pairs: [(String, (Int, Int))]) -> Int { var s = 0; for (_, (a, b)) in pairs { s += a * b }; return s }

func main() -> Int32 {
    var grid = Grid(cells: [1, 2, 3, 4], width: 2)
    print(grid[1, 0], grid[0, 1], grid[3], grid["w"])
    grid[1, 1] = 9
    grid[0, 0] += 5
    print(grid.cells)
    let r = Registry()
    r["a"] = 3
    r["a"] += 1
    print(r["a"], r["b"], Registry[21])
    var m = Matrix(rows: [[1, 2], [3, 4]])
    m[0] = [5, 6]
    m[1][0] = 7
    m[1].append(8)
    print(m.rows, m[1])
    print(f(((1, 2), 3)), g([("a", (2, 3)), ("b", (4, 5))]))

    var b = Board()
    print(b[at: 1].tag, b[tag: "a"] ?? -1, b[tag: "z"] ?? -1)
    b[at: 0].n += 10
    b[at: 1].tag += "!"
    b[at: 1] = Cell(n: 5, tag: "c")
    b.double(0)
    print(b.cells, b.first().n, b[sum: 0, 1])
    let fixed = Board()
    print(fixed[at: 0].n)
    print(Suit.spades[10], Suit.hearts[3])
    let bag = Bag()
    bag.items = ["apple", "avocado", "banana"]
    bag[2] += "s"
    bag[0] = "apricot"
    print(bag[0], bag[2], bag[prefix: "a"])
    return Int32(grid[0, 0])
}
