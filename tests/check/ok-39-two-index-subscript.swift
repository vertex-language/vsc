struct Grid {
    var cells: [[Int]] = []
    subscript(row: Int, col: Int) -> Int {
        return cells[row][col]
    }
}
