// §6.2 / §6.4 Ownership, Borrowing, Consuming, and Non-copyable types

struct FileDescriptor: ~Copyable {
    private var rawFD: Int

    init(rawFD: Int) {
        self.rawFD = rawFD
    }

    deinit {
        // cleanup file descriptor
    }

    borrowing func getRaw() -> Int {
        return self.rawFD
    }

    mutating func update(newFD: Int) {
        self.rawFD = newFD
    }

    consuming func close() {
        discard self
    }
}

func transferOwnership(fd: consuming FileDescriptor) {
    let closed = consume fd
    closed.close()
}

func inspectFD(fd: borrowing FileDescriptor) -> Int {
    return fd.getRaw()
}

func duplicateCopyable(val: [Int]) -> [Int] {
    let copied = copy val
    return copied
}

struct ResourcePool: ~Copyable {
    var primary: FileDescriptor

    consuming func decommission() {
        let fd = consume primary
        fd.close()
        discard self
    }
}
