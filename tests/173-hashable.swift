// Hashable types as Set elements and Dictionary keys.
struct Cell: Hashable { let row, col: Int }
enum Color: Hashable { case red, custom(Int) }
var visited: Set<Cell> = []
for (r, c) in [(0, 0), (0, 1), (0, 0), (2, 2), (0, 1)] {
    visited.insert(Cell(row: r, col: c))
}
print(visited.count, visited.contains(Cell(row: 2, col: 2)))
var names: [Color: String] = [.red: "red", .custom(7): "seven"]
names[.custom(7)] = "SEVEN"
print(names.count, names[.custom(7)]!, names[.custom(8)] as Any)
