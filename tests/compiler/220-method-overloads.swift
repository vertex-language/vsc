// Methods may share a name, told apart by their labels and argument
// types: a call picks the one its arguments fit, leaving out parameters
// that have defaults. A static method named without its type, inside
// another, is called on the type -- there is no self to pass.
struct Endpoint {
    var host: String
    var port: Int
}

struct Listener {
    let port: Int

    init(port: Int) {
        self.port = port
    }

    static func bind(_ address: String, backlog: Int = 1) -> Listener {
        return bind(host: address, port: address.count * 10 + backlog)
    }

    static func bind(host: String, port: Int) -> Listener {
        return Listener(port: port + host.count)
    }

    static func bind(endpoint: Endpoint) -> Listener {
        return Listener.bind(host: endpoint.host, port: endpoint.port)
    }

    func send(_ n: Int) -> Int {
        return n + port
    }

    func send(_ s: String) -> Int {
        return s.count + port
    }

    func send(bytes: Int, times: Int) -> Int {
        return send(bytes * times)
    }
}

func main() -> Int32 {
    let a = Listener.bind("tcp")
    let b = Listener.bind(host: "h", port: 7)
    let c = Listener.bind(endpoint: Endpoint(host: "abc", port: 20))
    var total = a.port + b.port + c.port
    total += a.send(1) + b.send("four") + c.send(bytes: 2, times: 3)
    return Int32(total % 251)
}
