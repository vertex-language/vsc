// The error a catch binds is an `any Error` held in memory. Interpolating
// it copies it out for String(describing:) and leaves the binding intact,
// so it can be described again, printed and rethrown afterwards.

enum ParseError: Error {
    case empty
    case badByte(Int)
}

struct Timeout: Error {
    var seconds: Int
}

func parse(_ n: Int) throws -> Int {
    if n == 0 { throw ParseError.empty }
    if n < 0 { throw ParseError.badByte(-n) }
    if n > 100 { throw Timeout(seconds: n) }
    return n * 2
}

func wrapped(_ n: Int) throws -> Int {
    do {
        return try parse(n)
    } catch {
        print("wrapped: \(error)")
        throw error
    }
}

func main() -> Int32 {
    var lines: [String] = []
    for n in [0, -7, 250, 4] {
        do {
            let v = try parse(n)
            lines.append("ok \(v)")
        } catch let e {
            lines.append("caught \(e) (again: \(e))")
            print(e)
        }
    }
    do {
        _ = try wrapped(-3)
    } catch {
        lines.append("outer \(error)")
    }
    for l in lines { print(l) }
    return Int32(lines.count)
}
