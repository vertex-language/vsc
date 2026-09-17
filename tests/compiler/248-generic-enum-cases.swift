// A case of a generic enum made through the enum's bare name says which
// instance by what it carries, and a case with nothing to carry named on
// an instance is a value of that instance. The enum's own methods switch
// over self for each instance they are called on.
enum Maybe<T> {
    case some(T)
    case none

    func or(_ fallback: T) -> T {
        switch self {
        case .some(let v):
            return v
        case .none:
            return fallback
        }
    }

    func isSome() -> Bool {
        if case .some = self {
            return true
        }
        return false
    }
}

func main() -> Int32 {
    let a = Maybe.some(5)
    let b = Maybe<Int>.none
    let c = Maybe.some("a string long enough to live on the heap")
    var total = a.or(1) + b.or(2) + c.or("").count
    if a.isSome() && !b.isSome() {
        total += 100
    }
    return Int32(total % 251)
}
