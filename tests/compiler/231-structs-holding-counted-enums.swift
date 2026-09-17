// A struct may hold an enum whose cases carry counted things: copying the
// struct retains what the enum's active case carries, and dropping it
// releases that -- in registers, in an array's storage, and in a class
// that holds the struct -- so every object is freed once.
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

enum Address {
    case v4(ip: String, port: Int)
    case named(Handle)
    case none
}

struct Listener {
    let address: Address
    let fd: Int32
}

final class Server {
    var listener: Listener

    init(_ l: Listener) {
        listener = l
    }
}

func weigh(_ l: Listener) -> Int {
    switch l.address {
    case .v4(let ip, let port):
        return ip.count + port
    case .named(let h):
        return h.id
    case .none:
        return 1
    }
}

func run() -> Int {
    var total = 0
    let a = Listener(address: .named(Handle(id: 5)), fd: 3)
    let b = a
    var ip = "10.0"
    ip += ".0.1"
    let c = Listener(address: .v4(ip: ip, port: 80), fd: 4)
    total += weigh(a) + weigh(b) + weigh(c)
    var list = [a, c, Listener(address: .named(Handle(id: 7)), fd: 5)]
    list.append(b)
    for l in list {
        total += weigh(l)
    }
    let server = Server(Listener(address: .named(Handle(id: 100)), fd: 9))
    total += weigh(server.listener)
    let wc = weigh(c)
    let wa = weigh(a)
    let get = { wc + wa }
    total += get()
    return total
}

func main() -> Int32 {
    let t = run()
    return Int32((t + freed) % 251)
}
