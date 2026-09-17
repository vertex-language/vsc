// A stored property's initializer runs before self exists, so a name in it
// that is an instance member of the type is an error -- even where a type
// of the same name is declared outside it.
struct ConnectionState {
    var complete = false
}

struct Conn {
    var state: ConnectionState = ConnectionState()

    func ConnectionState() -> ConnectionState {
        return self.state
    }
}
