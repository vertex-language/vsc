// A struct holds optionals of things that own references -- a String, a
// class instance, an array, a closure -- and a copy of the struct retains
// what each holds, while none of them holds nothing. What they own is let
// go when the struct is, and when a field is set back to nil.
var freed = 0

final class Tracker {
    let id: Int
    init(id: Int) {
        self.id = id
    }
    deinit {
        freed += 1
    }
}

struct Holder {
    var nickname: String? = nil
    var owner: Tracker? = nil
    var values: [Int]? = nil
    var callback: (() -> Int)? = nil
    var count: Int = 1
}

func fill() -> Int {
    var h = Holder()
    var total = h.count
    if h.nickname == nil {
        total += 1
    }
    h.nickname = "a nickname long enough to live on the heap"
    h.owner = Tracker(id: 7)
    h.values = [1, 2, 3]
    let base = 5
    h.callback = { base * 2 }
    let copy = h
    if let n = copy.nickname {
        total += n.count
    }
    if let o = copy.owner {
        total += o.id
    }
    if let v = h.values {
        total += v.count
    }
    if let f = h.callback {
        total += f()
    }
    h.owner = nil
    return total
}

func main() -> Int32 {
    let t = fill()
    return Int32(t + freed * 100)
}
