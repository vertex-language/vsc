// A conditional whose arms are Strings: each arm hands the join a
// value the join then owns, round a loop as well as once, and from a
// parameter the function only borrows.

func pick(_ yes: Bool, _ a: String, _ b: String) -> String {
    return yes ? a : b
}

func describe(_ x: Any) -> Int32 {
    return 1
}

func main() -> Int32 {
    let long = "a string long enough to live on the heap" + "!"
    var total = 0
    var i = 0
    while i < 1000 {
        let s = i % 2 == 0 ? long + "x" : "short"
        let t = pick(i % 3 == 0, s, long)
        total += t.count
        i += 1
    }
    let described = describe(total > 0 ? "yes" : "no")
    return Int32(total % 200) + described
}
