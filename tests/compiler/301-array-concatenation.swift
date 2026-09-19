// `a + b` of two arrays is a new array: a's elements, then b's. Neither
// operand changes, and an array literal on either side -- `[]` too --
// takes the other side's element type.

struct P { var s: String }
func main() -> Int32 {
    let key = [1, 2]
    var counter = 3
    let k2 = key + [counter]
    counter += 1
    let big = [0] + key + key + []
    var names = ["a"]
    let more = names + ["b", "c"]
    names.append("z")
    let ps = [P(s: "x")] + [P(s: "y")]
    let empty: [Int] = [] + []
    print(k2, big, names, more, ps.count, ps[1].s, empty.count)
    return Int32(big.count)
}
