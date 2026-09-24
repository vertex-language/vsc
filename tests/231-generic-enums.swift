// A generic enum with payloads of its parameters.
enum Either<L, R> {
    case left(L), right(R)
    func map<T>(_ f: (R) -> T) -> Either<L, T> {
        switch self {
        case .left(let l): return .left(l)
        case .right(let r): return .right(f(r))
        }
    }
    var description: String {
        switch self {
        case .left(let l): return "left(\(l))"
        case .right(let r): return "right(\(r))"
        }
    }
}
let xs: [Either<String, Int>] = [.right(2), .left("oops"), .right(5)]
print(xs.map { $0.map { $0 * 10 }.description })
