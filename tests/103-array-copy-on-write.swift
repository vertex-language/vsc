// Arrays are values: a copy is independent of the original.
var a = [1, 2, 3]
var b = a
b.append(4)
b[0] = 100
print(a, b)
func modify(_ xs: [Int]) -> [Int] {
    var copy = xs
    copy[0] = -1
    return copy
}
print(modify(a), a)
var grid = [[0, 0], [0, 0]]
let saved = grid
grid[1][0] = 5
print(grid, saved)
