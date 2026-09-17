// An [Any] holds each element in an existential: a literal of Ints,
// Strings, Doubles and class instances, and values appended to it. What
// the elements own is let go with the array.
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

func fill() -> Int {
    var items: [Any] = [1, "two", 3.0, Tracker(id: 5)]
    items.append(4)
    items.append("a string long enough to live on the heap")
    items.append(Tracker(id: 6))
    return items.count
}

func main() -> Int32 {
    let n = fill()
    return Int32(n * 10 + freed)
}
