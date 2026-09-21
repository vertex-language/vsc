// A function that returns Bool (an i1 result) can still throw: the error
// path needs a zero of every result register, and i1 has no plain literal.

enum E: Error { case bad }

func isPositive(_ x: Int) throws -> Bool {
    if x == 0 { throw E.bad }
    return x > 0
}

func check(_ x: Int) -> String {
    do {
        return try isPositive(x) ? "pos" : "neg"
    } catch {
        return "err"
    }
}

func main() -> Int32 {
    print(check(5))
    print(check(-3))
    print(check(0))
    return 0
}
