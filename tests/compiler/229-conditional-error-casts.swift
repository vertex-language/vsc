// `error as? T` in a catch-all is the error's value when it is exactly a
// T, and nil when it is not -- whether the error fits its existential's
// buffer or is boxed, and whether its type has one case or several.
enum NetError: Error {
    case refused(code: Int32, message: String)
    case closed
}

enum OtherError: Error {
    case bad
}

func open(_ n: Int) throws -> Int {
    if n == 0 {
        var m = "no"
        m += " route"
        throw NetError.refused(code: 7, message: m)
    }
    if n == 1 {
        throw OtherError.bad
    }
    if n == 2 {
        throw NetError.closed
    }
    return n
}

func main() -> Int32 {
    var total = 0
    for n in [0, 1, 2, 5] {
        do {
            total += try open(n)
        } catch {
            if let net = error as? NetError {
                switch net {
                case .refused(let c, let m):
                    total += Int(c) + m.count * 10
                case .closed:
                    total += 100
                }
            } else if let other = error as? OtherError {
                switch other {
                case .bad:
                    total += 1000
                }
            }
        }
    }
    return Int32(total % 251)
}
