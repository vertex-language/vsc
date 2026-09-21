// A struct initializer declared 'throws' can fail before self is complete;
// callers reach it with 'try', and the error propagates out of an
// enclosing throwing function like any throwing call.

enum MakeError: Error { case negative }

struct Positive {
    var value: Int
    init(_ v: Int) throws {
        if v < 0 { throw MakeError.negative }
        self.value = v
    }
}

func make(_ v: Int) -> String {
    do {
        let p = try Positive(v)
        return "ok \(p.value)"
    } catch {
        return "fail"
    }
}

print(make(7))
print(make(-2))

func makeTwo(_ a: Int, _ b: Int) throws -> Int {
    let x = try Positive(a)
    let y = try Positive(b)
    return x.value + y.value
}
do { print(try makeTwo(4, 5)) } catch { print("two failed") }
do { print(try makeTwo(4, -5)) } catch { print("two failed") }
