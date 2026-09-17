// A closure passed to Task may declare constants and use them. Among
// overloads that differ by label and by async, the label decides first and
// async second -- also when the argument is a leading-dot member whose type
// only the parameter can say. A closure that does not await is synchronous
// where it chooses, even passed where an async one is wanted.
struct Span {
    let n: Int64
    static func ms(_ n: Int64) -> Span { return Span(n: n) }
}
struct Moment { let n: Int64 }

var log: [Int64] = []

func wait(_ s: Span) { log.append(s.n) }
func wait(_ s: Span) async throws { log.append(s.n * 10) }
func wait(until m: Moment) { log.append(m.n * 100) }
func wait(until m: Moment) async throws { log.append(m.n * 1000) }

func main() async -> Int32 {
    var failures: Int32 = 0
    let t = Task { () async -> Void in
        let first = Int64(1)
        try? await wait(.ms(first))
    }
    await t.value
    try? await wait(.ms(2))
    try? await wait(until: Moment(n: 3))
    let u = Task {
        let k = Int64(4)
        wait(Span(n: k))
    }
    await u.value
    if log != [10, 20, 3000, 4] { failures += 1 }
    print(failures == 0 ? "ok" : "failed")
    return failures
}
