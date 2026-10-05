// §6.2 Non-Escapable, Non-Copyable, and Suppressed Conformances (SE-0390, SE-0427)

struct ByteSpan: ~Escapable {
    private var baseAddress: Int
    private var count: Int

    init(address: Int, count: Int) {
        self.baseAddress = address
        self.count = count
    }

    borrowing func read(at index: Int) -> UInt8 {
        guard index >= 0 && index < count else { return 0 }
        return 0xFF
    }
}

struct LinearBuffer<Element>: ~Copyable {
    private var storage: [Element]

    init(initial: [Element]) {
        self.storage = initial
    }

    deinit {
        // clean up
    }

    consuming func drain() -> [Element] {
        let items = self.storage
        discard self
        return items
    }
}

func exchange<T: ~Copyable>(_ first: inout T, _ second: inout T) {
    // generic over ~Copyable
}
