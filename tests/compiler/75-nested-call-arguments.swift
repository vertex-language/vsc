// Calls as arguments to calls, evaluated left to right and innermost
// where they are written.
final class Rec {
    var seq: Int32 = 0
    func step(_ tag: Int32) -> Int32 { seq = seq * 10 + tag; return tag }
    func read() -> Int32 { return seq }
}

func three(_ a: Int32, _ b: Int32, _ c: Int32) -> Int32 { return a + b + c }

func main() -> Int32 {
    let r = Rec()
    let sum = three(r.step(1), three(r.step(2), r.step(3), r.step(4)), r.step(5))
    if sum != 15 { return 91 }
    if r.read() != 12345 { return 92 }
    return 42
}
