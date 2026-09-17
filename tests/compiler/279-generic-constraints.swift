// What a generic function may do with a parameter its constraints
// describe: call the requirements, static ones included, use the
// operators the protocols declare and those that come with them, and
// keep values of it in the collections that need them Hashable.

protocol Shape {
    func area() -> Int
    static func unit() -> Self
    var name: String { get }
}

struct Square: Shape {
    var side: Int
    func area() -> Int { return side * side }
    static func unit() -> Square { return Square(side: 1) }
    var name: String { return "square" }
}

final class Circle: Shape {
    let r: Int
    init(r: Int) { self.r = r }
    func area() -> Int { return 3 * r * r }
    static func unit() -> Circle { return Circle(r: 1) }
    var name: String { return "circle" }
}

func total<T: Shape>(_ xs: [T]) -> Int {
    var n = T.unit().area()
    for x in xs { n += x.area() }
    return n
}

func describe<T: Shape>(_ x: T) -> String { return x.name + " of \(x.area())" }

func contains<T: Equatable>(_ xs: [T], _ e: T) -> Bool {
    for x in xs { if x == e { return true } }
    return false
}

func differs<T: Equatable>(_ a: T, _ b: T) -> Bool { return a != b }

func largest<T: Comparable>(_ xs: [T]) -> T {
    var best = xs[0]
    for x in xs where x > best { best = x }
    return best
}

func ordered<T: Comparable>(_ a: T, _ b: T) -> Bool { return a <= b && !(a >= b) || a == b }

func distinct<T: Hashable>(_ xs: [T]) -> Int {
    var seen = Set<T>()
    for x in xs { seen.insert(x) }
    return seen.count
}

func tally<K: Hashable>(_ xs: [K]) -> [K: Int] {
    var out: [K: Int] = [:]
    for x in xs { out[x] = (out[x] ?? 0) + 1 }
    return out
}

struct Point: Hashable { var x: Int; var y: Int }
struct Version: Comparable {
    var major: Int
    var minor: Int
    static func < (a: Version, b: Version) -> Bool {
        return a.major < b.major || a.major == b.major && a.minor < b.minor
    }
}
enum Direction { case north, south, east }

func check(_ ok: Bool, _ n: Int32) -> Int32 { return ok ? 0 : n }

func main() -> Int32 {
    var failed: Int32 = 0
    failed += check(total([Square(side: 2), Square(side: 3)]) == 14, 1)
    failed += check(total([Circle(r: 2)]) == 15, 2)
    failed += check(describe(Circle(r: 1)) == "circle of 3", 3)

    failed += check(contains([1, 2, 3], 2), 4)
    failed += check(!contains(["a", "b"], "c"), 5)
    failed += check(contains([Point(x: 1, y: 2)], Point(x: 1, y: 2)), 6)
    failed += check(contains([Direction.north, .east], .east), 7)
    failed += check(!contains([Direction.north], .south), 8)
    failed += check(differs(1.5, 2.5) && !differs("x", "x"), 9)
    failed += check(differs(Point(x: 0, y: 0), Point(x: 0, y: 1)), 10)

    failed += check(largest([3, 9, 2]) == 9, 11)
    failed += check(largest(["pear", "apple", "plum"]) == "plum", 12)
    let v = largest([Version(major: 1, minor: 9), Version(major: 2, minor: 0), Version(major: 1, minor: 10)])
    failed += check(v.major == 2 && v.minor == 0, 13)
    failed += check(ordered(1, 2) && ordered(Version(major: 1, minor: 1), Version(major: 1, minor: 1)), 14)

    failed += check(distinct([1, 2, 2, 3, 1]) == 3, 15)
    failed += check(distinct(["a", "a"]) == 1, 16)
    failed += check(distinct([Point(x: 1, y: 1), Point(x: 1, y: 1), Point(x: 2, y: 1)]) == 2, 17)
    failed += check(distinct([Direction.north, .north, .south]) == 2, 18)
    let t = tally(["x", "y", "x"])
    failed += check(t["x"] == 2 && t["y"] == 1 && t.count == 2, 19)
    failed += check(tally([Direction.east, .east])[.east] == 2, 20)

    var dirs = Set<Direction>()
    dirs.insert(.north)
    dirs.insert(.north)
    failed += check(dirs.count == 1 && dirs.contains(.north), 21)

    print(largest([3, 9, 2]), largest(["pear", "apple", "plum"]), describe(Square(side: 4)))
    return failed
}
