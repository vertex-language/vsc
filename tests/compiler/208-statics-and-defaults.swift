// A static stored property has storage of its own, filled by its
// initializer the first time it is read -- one static's initializer may
// read another -- and a property's default value is there before an
// initializer's body runs, whether or not the body assigns it.
struct Options {
    var backlog: Int = 128
    var reuse: Bool = true
    var name: String = "options"

    init() {}

    init(backlog: Int) {
        self.backlog = backlog
    }

    static let standard = Options()
    static let big = Options(backlog: standard.backlog * 4)

    func isStandard() -> Bool {
        return backlog == Options.standard.backlog
    }
}

struct Point {
    var x: Int
    var y: Int
    static let origin = Point(x: 3, y: 4)
}

final class Registry {
    static var created = 7
    static let name = "registry"
}

final class Counter {
    var count: Int = 5
    var label: String = "counter"
    init() {}
}

final class Partial {
    var a: Int = 1
    var b: Int = 2
    init(b: Int) {
        self.b = b
    }
}

func main() -> Int32 {
    var total = 0
    total += Options.standard.backlog
    if Options.standard.reuse {
        total += 1
    }
    total += Options.big.backlog / 16
    if Options.big.name == "options" {
        total += 10
    }
    if Options.standard.isStandard() && !Options.big.isStandard() {
        total += 100
    }
    total += Point.origin.x * Point.origin.y
    total += Registry.created
    if Registry.name == "registry" {
        total += 1000
    }
    let local = Options()
    total += local.backlog
    let counter = Counter()
    total += counter.count * 3
    if counter.label == "counter" {
        total += 20
    }
    let partial = Partial(b: 30)
    total += partial.a + partial.b
    return Int32(total % 251)
}
