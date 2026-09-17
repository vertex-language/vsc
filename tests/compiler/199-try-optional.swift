// try? turns a failure into nil and try! a success into its value.
enum Failure: Error {
    case negative
}

func half(_ x: Int) throws -> Int {
    if x < 0 { throw Failure.negative }
    return x / 2
}

func main() -> Int32 {
    var n = 0
    if let v = try? half(40) {
        n += v
    }
    if let w = try? half(-4) {
        n += w * 1000
    } else {
        n += 7
    }
    let x: Int? = try? half(-1)
    if x == nil {
        n += 3
    }
    n += try! half(10)
    return Int32(n)
}
