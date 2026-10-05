// inout parameters write back to the caller's variable.
func swapTwo(_ a: inout Int, _ b: inout Int) {
    let t = a
    a = b
    b = t
}
func bump(_ x: inout Int, by n: Int = 1) { x += n }
var p = 1, q = 2
swapTwo(&p, &q)
bump(&p)
bump(&q, by: 10)
print(p, q)
var arr = [1, 2, 3]
bump(&arr[1], by: 40)
print(arr)
