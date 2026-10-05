// Subscripts of a type's own: read-write, with labels, and with several parameters.
struct Matrix {
    let rows: Int, cols: Int
    var cells: [Double]
    init(rows: Int, cols: Int) {
        self.rows = rows
        self.cols = cols
        cells = Array(repeating: 0, count: rows * cols)
    }
    subscript(r: Int, c: Int) -> Double {
        get { cells[r * cols + c] }
        set { cells[r * cols + c] = newValue }
    }
    subscript(row r: Int) -> [Double] { Array(cells[(r * cols)..<((r + 1) * cols)]) }
}
var m = Matrix(rows: 2, cols: 3)
m[0, 1] = 1.5
m[1, 2] = 4
m[1, 2] += 1
print(m[0, 1], m[1, 2], m[row: 1])
