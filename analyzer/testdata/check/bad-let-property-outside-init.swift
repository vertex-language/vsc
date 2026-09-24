struct Socket {
    let fd: Int32

    init(fd: Int32) {
        self.fd = fd
    }

    mutating func reopen(_ other: Int32) {
        self.fd = other
    }
}
