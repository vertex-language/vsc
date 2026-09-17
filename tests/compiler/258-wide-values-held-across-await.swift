// A value too wide for registers, loaded out of a variable and taken apart
// into its scalars, and then still wanted after an await.
//
// In an ordinary function such a value is held both ways: as scalars for
// the code that reads its fields, and in storage when something wants its
// address. An async function that suspends is re-entered after the await
// in a fresh run of its body, and the scalars were registers in the run
// that returned. Only the storage survives -- it is in the task's frame --
// so the value has to be read from there. Reading the registers instead is
// a use of a value defined in a function that has already given up its
// thread.
enum Addr {
    case v4(ip: String, port: UInt16)
    case v6(ip: String, port: UInt16)
}

func text(_ a: Addr) -> String {
    switch a {
    case .v4(let ip, let port): return "\(ip):\(port)"
    case .v6(let ip, let port): return "[\(ip)]:\(port)"
    }
}

struct Conn {
    let local: Addr
    let peer: Addr
    let fd: Int32
    var readTimeout: Int32 = 0
    var writeTimeout: Int32 = 0
}

func tick() async -> Int32 {
    await Task.yield()
    return 1
}

func open(_ fd: Int32) async -> Conn {
    _ = await tick()
    return Conn(local: .v4(ip: "127.0.0.1", port: 1), peer: .v6(ip: "::1", port: 2), fd: fd)
}

func main() async -> Int32 {
    var c = await open(3)
    let before = text(c.peer).count          // "[::1]:2" is 7
    _ = await tick()
    c.readTimeout = 5
    let after = text(c.local).count          // "127.0.0.1:1" is 11
    _ = await tick()
    return c.fd + c.readTimeout + Int32(before + after)   // 3 + 5 + 18 = 26
}
