// Compiled by this compiler, linked against the library above.
import Bags

// What an array holds, read here rather than over there. Neither
// `count` nor `a[i]` is a field: an Array's storage is behind a
// reference this compiler never looks inside, so both are calls at
// the symbols libswiftCore exports. The subscript getter is generic
// and hands its element back through storage the caller set aside,
// which is why an Int32 arrives through a four-byte slot in x8.
func mine(_ a: [Int32]) -> Int32 {
    var t: Int32 = 0
    var i = 0
    while i < a.count {
        t += a[i]
        i += 1
    }
    return t
}

// `for x in a`, which is not an iterator here: the array is evaluated
// once, its count is read once, and the loop is an index walk -- the
// same program `swiftc -O` arrives at once it has specialized the
// iterator away.
func walk(_ a: [Int32]) -> Int32 {
    var t: Int32 = 0
    for x in a { t += x }
    return t
}

func firstOver(_ a: [Int32], _ limit: Int32) -> Int32 {
    for x in a {
        if x <= limit { continue }
        return x
    }
    return -1
}

func countUp(_ a: [Int32]) -> Int32 {
    var n: Int32 = 0
    for _ in a { n += 1 }
    return n
}

// Nested, and over a value the loop asked for once: a for-in over
// `upTo(3)` calls it a single time.
func grid(_ a: [Int32], _ b: [Int32]) -> Int32 {
    var t: Int32 = 0
    for x in a {
        for y in b {
            if y == 0 { break }
            t += x * y
        }
    }
    return t
}

// A variadic declared here rather than over there: the body is
// handed the array the caller built, and passes it on as one.
func mySum(_ xs: Int32...) -> Int32 { return total(xs) }

func mySize(_ tag: Int32, _ xs: Int32...) -> Int32 { return tag + size(xs) }

func main() -> Int32 {
    // Read here, and the same answer the library gives.
    let here: [Int32] = [10, 20, 30]
    if here.count != 3 { return 71 }
    if here[0] != 10 { return 72 }
    if here[2] != 30 { return 73 }
    if mine(here) != total(here) { return 74 }
    if mine([]) != 0 { return 75 }

    // Element types that are not four bytes.
    let reals = [1.5, 2.5]
    if reals[1] != 2.5 { return 76 }
    let wide: [Int64] = [1, 2, 3]
    if wide[2] != 3 { return 77 }
    let flags = [true, false, true]
    if flags[1] { return 78 }
    if !flags[2] { return 79 }

    if walk([10, 20, 12]) != 42 { return 61 }
    if walk([]) != 0 { return 62 }
    if firstOver([1, 5, 9], 4) != 5 { return 63 }
    if firstOver([1, 2], 9) != -1 { return 64 }
    if countUp(upTo(7)) != 7 { return 65 }
    if grid([1, 2], [3, 4]) != 21 { return 66 }
    if grid([1, 2], [0, 4]) != 0 { return 67 }
    if walk(upTo(5)) != 10 { return 68 }

    if mySum(40, 2) != 42 { return 81 }
    if mySum() != 0 { return 82 }
    if mySize(5, 1, 2, 3) != 8 { return 83 }

    if total([40, 2]) != 42 { return 91 }
    if size([1, 2, 3, 4]) != 4 { return 92 }

    // Empty, which allocates nothing and still has to be an array.
    if size([]) != 0 { return 93 }

    // One element, which is the case where the index arithmetic
    // never runs.
    if at([7], 0) != 7 { return 94 }

    // The elements are laid out at their stride, so reading the last
    // one back is what says the offsets were right.
    if at([10, 20, 30, 40, 50], 4) != 50 { return 95 }
    if at([10, 20, 30, 40, 50], 2) != 30 { return 96 }

    // Element types wider and narrower than a word, and one that is
    // not an integer at all.
    if totalWide([1000000000000, 2]) != 1000000000002 { return 97 }
    if totalReal([1.5, 2.25]) != 3.75 { return 98 }
    if trueCount([true, false, true, true]) != 3 { return 99 }

    // One handed back, which owns its storage: it is passed on, bound,
    // and let go of through Swift's own bridge-object release.
    if size(upTo(6)) != 6 { return 100 }
    let ns = upTo(5)
    if size(ns) != 5 { return 101 }
    if at(ns, 3) != 3 { return 102 }

    // A variadic parameter: the same array, built by the caller.
    if sum(40, 2) != 42 { return 103 }
    if sum() != 0 { return 104 }
    if sum(7) != 7 { return 105 }
    if after(10, 1, 2, 3) != 36 { return 106 }
    if after(10) != 0 { return 107 }

    return total([20, 20]) + at([1, 2], 1)
}
