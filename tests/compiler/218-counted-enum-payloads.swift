// A case may carry something counted: a String, an object, a closure, a
// tuple holding one. Copying the enum retains what its active case
// carries and dropping it releases that -- including in a switch arm
// that binds nothing, or the default -- so an object a case holds dies
// once the last enum holding it has gone, and not before.
var freed = 0

final class Token {
    let id: Int

    init(id: Int) {
        self.id = id
    }

    deinit {
        freed += id
    }
}

enum Event {
    case text(String)
    case token(Token)
    case action((Int) -> Int)
    case pair(String, Int)
    case empty
}

func weigh(_ e: Event) -> Int {
    switch e {
    case .text(let s):
        return s.count
    case .token(let t):
        return t.id
    case .action(let f):
        return f(2)
    case .pair(let s, let n):
        return s.count * n
    case .empty:
        return 1
    }
}

func kind(_ e: Event) -> Int {
    switch e {
    case .text:
        return 1
    case .token:
        return 2
    default:
        return 3
    }
}

func run() -> Int {
    var total = 0
    var name = "sock"
    name += "et"
    let base = 5
    let a = Event.text(name)
    let b = Event.token(Token(id: 100))
    let c = Event.action({ $0 * base })
    let d = Event.pair(name + "!", 3)
    let copy = b
    total += weigh(a) + weigh(b) + weigh(c) + weigh(d) + weigh(.empty)
    total += kind(a) + kind(copy) + kind(c) + kind(d) + kind(.empty)
    var slot = Event.token(Token(id: 20))
    total += weigh(slot)
    slot = .text("replaced")
    total += weigh(slot)
    return total
}

func main() -> Int32 {
    let total = run()
    return Int32((total + freed) % 251)
}
