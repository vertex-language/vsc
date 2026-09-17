// An error too wide for an existential's three words is boxed, and one
// whose cases carry counted things -- an object, a String -- has a value
// witness table that retains and releases what the active case carries.
// Every object an error held is freed once the error is gone.
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

func open(_ n: Int) throws -> Int {
    if n % 2 == 0 {
        throw NetError.lost(handle: Handle(id: n * 10), message: "gone")
    }
    if n == 3 {
        throw NetError.closed
    }
    return n
}

func run() -> Int {
    var total = 0
    for n in [2, 3, 4, 5] {
        do {
            total += try open(n)
        } catch let e as NetError {
            switch e {
            case .lost(let h, let m):
                total += h.id + m.count
            case .closed:
                total += 100
            }
        } catch {
            total += 1000
        }
    }
    return total
}

func main() -> Int32 {
    let total = run()
    return Int32((total + freed) % 251)
}
