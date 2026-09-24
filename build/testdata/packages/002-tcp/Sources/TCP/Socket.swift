import CTcp

// A connected socket.
public struct Socket {
    public let fd: Int32

    public init(fd: Int32) {
        self.fd = fd
    }

    public func isOpen() -> Bool {
        return fd >= 0
    }

    public func send(_ value: Int32) -> Bool {
        return tcp_send_int(fd, value) == 4
    }

    public func receive() -> Int32 {
        return tcp_receive_int(fd, -1)
    }

    public func close() {
        _ = tcp_close(fd)
    }
}

// A socket accepting connections on the loopback interface.
public struct Listener {
    public let socket: Socket

    public init() {
        socket = Socket(fd: tcp_listen_loopback())
    }

    public func port() -> Int32 {
        return tcp_port(socket.fd)
    }

    public func accept() -> Socket {
        return Socket(fd: tcp_accept(socket.fd))
    }
}

public func connect(port: Int32) -> Socket {
    return Socket(fd: tcp_connect_loopback(port))
}
