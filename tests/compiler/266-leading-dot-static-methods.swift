// A static method that makes a value of a type is called with a leading
// dot wherever that type is wanted: in an annotated let, as an argument,
// and as a return value. Overloads are told apart by their labels.
struct Span {
    let nanos: Int64
    static func seconds(_ n: Int64) -> Span { return Span(nanos: n * 1_000_000_000) }
    static func seconds(millis n: Int64) -> Span { return Span(nanos: n * 1_000_000) }
    static let zero = Span(nanos: 0)
}

func take(_ s: Span) -> Int64 { return s.nanos }
func half() -> Span { return .seconds(millis: 500) }

func main() -> Int32 {
    var failures: Int32 = 0
    let s: Span = .seconds(1)
    if s.nanos != 1_000_000_000 { failures += 1 }
    if take(.seconds(2)) != 2_000_000_000 { failures += 1 }
    if take(.seconds(millis: 3)) != 3_000_000 { failures += 1 }
    if take(.zero) != 0 { failures += 1 }
    if half().nanos != 500_000_000 { failures += 1 }
    print(failures == 0 ? "ok" : "failed")
    return failures
}
