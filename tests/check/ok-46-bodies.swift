struct Meter {
    var raw: Int = 0

    var doubled: Int { return raw * 2 }

    var scaled: Int {
        get { return raw * 3 }
        set { raw = newValue / 3 }
    }

    var watched: Int = 0 {
        willSet { _ = newValue }
        didSet { _ = oldValue }
    }

    init(raw: Int) {
        self.raw = raw
    }

    subscript(offset: Int) -> Int {
        return raw + offset
    }
}

class Resource {
    var open = true
    deinit { open = false }
}

func work() throws -> Int {
    defer { }
    outer: for i in [1, 2] {
        if i > 1 { break outer }
    }
    do {
        return try produce()
    } catch {
        _ = error
        return 0
    }
}

func produce() throws -> Int { return 1 }

enum Code: Int {
    case ok = 0
    case bad = 1
}
