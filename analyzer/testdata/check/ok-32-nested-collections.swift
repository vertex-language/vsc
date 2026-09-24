struct Matrix {
    var rows: [[Int]]
    func at(_ i: Int, _ j: Int) -> Int {
        return rows[i][j]
    }
}
