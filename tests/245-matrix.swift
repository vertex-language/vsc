// Matrices of Double: multiply, transpose, and an identity check.
typealias Matrix = [[Double]]
func multiply(_ a: Matrix, _ b: Matrix) -> Matrix {
    (0..<a.count).map { i in
        (0..<b[0].count).map { j in
            (0..<b.count).reduce(0.0) { $0 + a[i][$1] * b[$1][j] }
        }
    }
}
func transpose(_ m: Matrix) -> Matrix { (0..<m[0].count).map { j in m.map { $0[j] } } }
let a: Matrix = [[1, 2], [3, 4], [5, 6]]
let b: Matrix = [[0.5, -1], [2, 0.25]]
print(multiply(a, b))
print(transpose(a))
let rot: Matrix = [[0, -1], [1, 0]]
print(multiply(multiply(rot, rot), multiply(rot, rot)) == [[1, 0], [0, 1]])
