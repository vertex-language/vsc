// A case may bind what it matched and may guard it with a condition.
//
// `case let k` names the subject and matches whatever it is, so it is
// a default with a name -- nothing after it is reachable, and the
// continuation is not made at all where nothing reaches it.
//
// A where clause is read after the binding, because it is written
// about the names the pattern declares. The two together are the
// pattern's test and the condition, and-ed without a branch: neither
// has an effect to skip.
func classify(_ n: Int32) -> Int32 {
    switch n {
    case 0: return 1
    case let k where k < 0: return -k
    case 1..<10: return 2
    case 10...20 where n % 2 == 0: return 3
    case let k where k > 100: return k / 10
    default: return 9
    }
}

// A binding with nothing after it, which is the shape that used to
// leave a block with no predecessors behind.
func doubleIt(_ n: Int32) -> Int32 {
    switch n {
    case 0: return 0
    case let k: return k * 2
    }
}

func main() -> Int32 {
    let a = classify(0) + classify(-5) + classify(5) + classify(12)
    let b = classify(13) + classify(200) + classify(50)
    return a + b + doubleIt(0) + doubleIt(4)
}
