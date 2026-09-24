struct Outer {
    enum Kind {
        case first
        case second
    }

    struct Inner {
        var depth = 0
        func doubled() -> Int { return depth * 2 }
    }

    var kind: Kind = .first
    var inner = Inner()

    func use() -> Int {
        return inner.doubled()
    }
}

class Holder {
    struct Slot {
        var n = 0
    }
    var slot = Slot()
}
