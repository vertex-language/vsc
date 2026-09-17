// A catch-all's error is a copy of the thrown one: copying it retains what
// the error holds, and the copy is destroyed when the clause ends, so every
// object an error carried is freed exactly once.
var freed = 0

final class Handle {
    let id: Int

    init(id: Int) {
        self.id = id
    }

    deinit {
        freed += id
    }
}

enum NetError: Error {
    case lost(handle: Handle, message: String)
    case closed
}

enum Small: Error {
    case held(Handle)
}

func open(_ n: Int) throws -> Int {
    if n == 0 {
        throw NetError.lost(handle: Handle(id: 10), message: "gone")
    }
    if n == 1 {
        throw Small.held(Handle(id: 200))
    }
    if n == 2 {
        throw NetError.closed
    }
    return n
}

func run() -> Int {
    var total = 0
    for n in [0, 1, 2, 3] {
        do {
            total += try open(n)
        } catch {
            if let e = error as? NetError {
                switch e {
                case .lost(let h, let m):
                    total += h.id + m.count
                case .closed:
                    total += 1000
                }
            } else if let s = error as? Small {
                switch s {
                case .held(let h):
                    total += h.id * 2
                }
            }
        }
    }
    return total
}

func main() -> Int32 {
    let total = run()
    return Int32((total + freed) % 251)
}
