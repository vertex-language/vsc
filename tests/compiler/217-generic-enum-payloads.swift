// A generic enum's cases carry its arguments: .failure of
// Outcome<Int, NetError> takes a NetError, so an implicit member inside
// it -- .closed, .refused(port) -- resolves against NetError.
enum NetError: Error {
    case refused(Int)
    case closed
}

enum Outcome<Value, Failure: Error> {
    case success(Value)
    case failure(Failure)
}

func connect(_ port: Int) -> Outcome<Int, NetError> {
    if port == 0 {
        return .failure(.closed)
    }
    if port < 1024 {
        return .failure(.refused(port))
    }
    return .success(port + 1)
}

func label(_ o: Outcome<UInt8, NetError>) -> Int {
    switch o {
    case .success(let s):
        return Int(s)
    case .failure(let e):
        switch e {
        case .refused(let q):
            return q
        case .closed:
            return 7
        }
    }
}

func main() -> Int32 {
    var total = 0
    for p in [0, 80, 8080] {
        switch connect(p) {
        case .success(let fd):
            total += fd
        case .failure(let e):
            switch e {
            case .refused(let q):
                total += q * 2
            case .closed:
                total += 7
            }
        }
    }
    total += label(.success(4)) + label(.failure(.refused(40)))
    return Int32(total % 251)
}
