struct Socket {
    let fd: Int32
    let port: Int32

    init(fd: Int32, port: Int32) {
        self.fd = fd
        self.port = port
    }
}

struct Listener {
    let socket: Socket

    init() {
        socket = Socket(fd: 3, port: 80)
    }
}

final class Connection {
    let id: Int32
    var open: Bool

    init(id: Int32) {
        self.id = id
        open = true
    }
}

func use() -> Int32 {
    let l = Listener()
    let c = Connection(id: 7)
    return l.socket.fd + l.socket.port + c.id
}
