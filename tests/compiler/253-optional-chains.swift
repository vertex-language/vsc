// A chain through an optional reads one member of what it holds, when it
// holds something: a String's count, a class instance's properties, and a
// plain struct's stored ones. What the optional owns is let go on the arm
// that had it, and the answer is an optional of what was read.
var freed = 0

final class Tracker {
    let id: Int
    var name: String = "a tracker name long enough to live on the heap"
    init(id: Int) {
        self.id = id
    }
    deinit {
        freed += 1
    }
}

struct Point {
    var x: Int
    var y: Int
}

struct Profile {
    var nickname: String? = nil
}

func run() -> Int {
    var p = Profile()
    var total = p.nickname?.count ?? 1
    p.nickname = "a nickname long enough to live on the heap"
    total += p.nickname?.count ?? 0
    let point: Point? = Point(x: 3, y: 4)
    total += (point?.x ?? 0) + (point?.y ?? 0)
    let nowhere: Point? = nil
    total += nowhere?.x ?? 100
    let t: Tracker? = Tracker(id: 7)
    let label = t?.name ?? ""
    total += (t?.id ?? 0) + label.count
    let gone: Tracker? = nil
    total += (gone?.name ?? "").count
    return total
}

func main() -> Int32 {
    let n = run()
    return Int32((n + freed * 1000) % 251)
}
