// An initializer is a function: it may branch, loop, and call before
// it has finished filling self in.
struct Summary {
    var total: Int32
    var count: Int32

    init(upTo n: Int32) {
        var t: Int32 = 0
        var c: Int32 = 0
        for i in 0..<n {
            if Int32(i) % 3 == 0 { continue }
            t = t + Int32(i)
            c = c + 1
        }
        total = t
        count = c
    }
}

func main() -> Int32 {
    let s = Summary(upTo: 10)
    // 1+2+4+5+7+8 = 27, six of them.
    if s.total != 27 { return 91 }
    if s.count != 6 { return 92 }
    return s.total + s.count + 9
}
