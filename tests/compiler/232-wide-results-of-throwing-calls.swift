// A call that may throw and returns a value too wide for registers hands
// it back through storage set aside for it. The value reaches the normal
// path as its scalars, and goes on by address where a call wants it so.
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
}

struct Stream {
    let local: Address
    let peer: Address
    let fd: Int32
}

enum OpenError: Error {
    case refused(Int)
}

func accept(_ n: Int) throws -> Stream {
    if n == 0 {
        throw OpenError.refused(7)
    }
    var ip = "10.0.0."
    ip += "\(n)"
    return Stream(local: .v4(ip: ip, port: 80), peer: .named(Handle(id: n * 10)), fd: Int32(n))
}

func weigh(_ s: Stream) -> Int {
    var total = Int(s.fd)
    switch s.local {
    case .v4(let ip, let port):
        total += ip.count + port
    case .named(let h):
        total += h.id
    }
    switch s.peer {
    case .v4(let ip, _):
        total += ip.count
    case .named(let h):
        total += h.id
    }
    return total
}

func relay(_ n: Int) throws -> Int {
    let s = try accept(n)
    return weigh(s)
}

func run() -> Int {
    var total = 0
    for n in [0, 1, 2] {
        do {
            let s = try accept(n)
            total += weigh(s)
        } catch {
            total += 1000
        }
    }
    total += (try? relay(3)) ?? 5000
    total += (try? relay(0)) ?? 5000
    return total
}

func main() -> Int32 {
    let t = run()
    return Int32((t + freed) % 251)
}
