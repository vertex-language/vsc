// Subscripts a type declares: read and written, with and without
// labels, static, on a class, in an extension, and through the type's
// own `self`.
struct Grid {
    var cells: [Int]
    subscript(x: Int, y: Int) -> Int {
        get { return cells[y * 2 + x] }
        set { cells[y * 2 + x] = newValue }
    }
    subscript(at i: Int) -> Int { return cells[i] }
    static subscript(n: Int) -> Int { return n * 2 }
    mutating func bump() { self[0, 0] += 1 }
}
extension Grid {
    subscript(name: String) -> String { return name }
}
final class Bag {
    var items: [String] = []
    subscript(i: Int) -> String {
        get { return items[i] }
        set { items[i] = newValue }
    }
}

func f() -> Int {
    var g = Grid(cells: [1, 2, 3, 4])
    g[1, 0] = 9
    g[0, 1] += 1
    g.bump()
    let bag = Bag()
    bag.items = ["a"]
    bag[0] = "b"
    bag[0] += "c"
    return g[0, 0] + g[at: 1] + Grid[3] + g["n"].count + bag[0].count
}
