// A payload pattern may leave a value out with `_`, and a case may name
// a static property -- `case Code.refused:` -- to match what it equals.
// An enum too wide for registers, like Address with its v6 case, is
// built where it is kept and switched on from there.
enum Code {
    static var ok: Int32 { return 0 }
    static var refused: Int32 { return -2 }
    static var timedOut: Int32 { return -3 }
}

enum Address {
    case v4(ip: String, port: UInt16)
    case v6(ip: String, port: UInt16, scope: Int)
}

func port(_ a: Address) -> Int {
    switch a {
    case .v4(_, let p):
        return Int(p)
    case .v6(let ip, _, _):
        return ip.count
    }
}

func name(_ code: Int32) -> Int {
    switch code {
    case Code.ok:
        return 1
    case Code.refused:
        return 20
    case Code.timedOut:
        return 300
    default:
        return 4000
    }
}

func main() -> Int32 {
    var ip = "10.0.0"
    ip += ".1"
    let total = port(.v4(ip: ip, port: 80)) + port(.v6(ip: ip, port: 1, scope: 2)) +
        name(0) + name(-2) + name(-3) + name(7)
    return Int32(total % 251)
}
