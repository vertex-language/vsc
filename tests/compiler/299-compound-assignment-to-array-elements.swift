// a[i] op= v changes the element where it is: the index is evaluated once,
// the operand before the element is reached, and the array is not copied
// for each one -- a loop over many elements stays linear.

final class Box { var xs = [1, 2, 3] }
var calls = 0
func idx(_ i: Int) -> Int { calls += 1; return i }
func main() -> Int32 {
    var a = [1, 2, 3]
    let before = a
    a[0] += 10
    a[idx(1)] *= a.count
    a[2] -= a[0]
    var s = ["x", "y"]
    s[1] += "z"
    s[0] += s[1]
    var rows: [[Int]] = [[1], []]
    rows[1] += [4, 5]
    rows[0] += rows[1]
    var d: [String: Int] = [:]
    d["k", default: 0] += 3
    let b = Box()
    b.xs[1] += 40
    var f: [Double] = [1.5]
    f[0] /= 2
    var big = [Int](repeating: 0, count: 100000)
    for k in 0..<100000 { big[k] += k % 7 }
    var sum = 0
    for k in 0..<100000 { sum += big[k] }
    print(sum)
    print(a, before, s, rows, d["k"]!, b.xs, f, calls)
    return Int32(a[0])
}
