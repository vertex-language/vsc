// A struct holds an optional of a plain value -- its payload and a tag
// byte -- beside counted fields: an array before it and a String after,
// which are found at their own offsets. The struct is made with defaults
// and memberwise, kept in a let or a var, read, and assigned through.
struct Record {
    var tags: [String] = []
    var score: Int? = nil
    var name: String = "record"
    var ratio: Double? = nil
}

struct Small {
    var a: Int?
    var b: Int
}

func describe(_ r: Record) -> Int {
    var total = r.tags.count + r.name.count
    if let s = r.score {
        total += s
    }
    if r.ratio != nil {
        total += 100
    }
    return total
}

func main() -> Int32 {
    var r = Record()
    var total = describe(r)
    r.tags.append("x")
    r.score = 4
    r.name = "a name long enough to live on the heap"
    total += describe(r)
    let fixed = Record(tags: ["a", "b"], score: nil, name: "n", ratio: 0.5)
    total += describe(fixed)
    var small = Small(a: nil, b: 1)
    small.a = 6
    total += (small.a ?? 0) + small.b
    let copy = small
    total += copy.a ?? 0
    return Int32(total % 251)
}
