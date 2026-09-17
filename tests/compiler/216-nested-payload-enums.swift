// A case may carry another enum that carries a value: the inner enum's
// words, tag and all, are the outer case's payload, and the outer tag
// sits past them. Taking the inner enum out leaves the outer tag behind,
// so wrapping it in a different enum sets that enum's tag alone.
enum NetError: Error {
    case refused(Int)
    case closed
}

enum Outcome {
    case success(Int)
    case failure(NetError)
}

func connect(_ port: Int) -> Outcome {
    if port == 0 {
        return .failure(.closed)
    }
    if port < 1024 {
        return .failure(.refused(port))
    }
    return .success(port + 1)
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
    return Int32((total + check()) % 251)
}

enum Other {
    case wrapped(NetError)
    case empty
    case done
}

func rewrap(_ o: Outcome) -> Other {
    switch o {
    case .failure(let e):
        return .wrapped(e)
    case .success:
        return .done
    }
}

func check() -> Int {
    var sum = 0
    for p in [0, 80, 8080] {
        switch rewrap(connect(p)) {
        case .wrapped(let e):
            switch e {
            case .refused(let q):
                sum += q
            case .closed:
                sum += 11
            }
        case .empty:
            sum += 1000
        case .done:
            sum += 5
        }
    }
    return sum
}
