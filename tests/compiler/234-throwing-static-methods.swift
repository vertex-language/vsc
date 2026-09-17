// A static method may throw: a call to it goes out on both edges, whether
// it is written with try in a do block, with try?, or from another static
// method of its type. What it returns in several words reaches the normal
// path as its fields.
enum BindError: Error {
    case refused(Int32)
}

enum Address {
    case v4(ip: String, port: UInt16)
    case none
}

struct Options {
    var backlog: Int32 = 16

    static let `default` = Options()
}

struct Listener {
    let address: Address
    let fd: Int32

    static func bind(host: String, port: UInt16, options: Options = .default) throws -> Listener {
        if port == 0 {
            throw BindError.refused(options.backlog)
        }
        return Listener(address: .v4(ip: host, port: port), fd: options.backlog + Int32(port))
    }

    static func bind(_ text: String, options: Options = .default) throws -> Listener {
        return try bind(host: text, port: UInt16(text.count), options: options)
    }
}

func weigh(_ l: Listener) -> Int {
    switch l.address {
    case .v4(let ip, let port):
        return ip.count + Int(port) + Int(l.fd)
    case .none:
        return Int(l.fd)
    }
}

func main() -> Int32 {
    var total = 0
    do {
        total += weigh(try Listener.bind(host: "local", port: 80))
        total += weigh(try Listener.bind("host", options: Options(backlog: 4)))
        _ = try Listener.bind(host: "x", port: 0)
    } catch BindError.refused(let n) {
        total += Int(n) * 100
    } catch {
        total += 7
    }
    if let l = try? Listener.bind("abc") {
        total += weigh(l)
    }
    return Int32(total % 251)
}
