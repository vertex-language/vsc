struct Payload: Sendable {
    var n = 0
}

struct Handle: ~Copyable {
    var n = 0
    deinit {}
}

func use(_ p: Payload) -> Int {
    return p.n
}
