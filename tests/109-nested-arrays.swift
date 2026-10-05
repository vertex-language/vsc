// Arrays of arrays: building, indexing, and flattening.
var grid: [[Int]] = []
for r in 0..<3 {
    var row: [Int] = []
    for c in 0..<4 { row.append(r * 4 + c) }
    grid.append(row)
}
print(grid, grid[2][3], grid.count, grid[0].count)
print(grid.flatMap { $0 }.reduce(0, +), grid.map { $0.reduce(0, +) })
let transposed = (0..<4).map { c in grid.map { $0[c] } }
print(transposed)
