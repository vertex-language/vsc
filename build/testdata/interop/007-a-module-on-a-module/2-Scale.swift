// The upper library. Its own interface says `import Units`, and its
// functions take a type declared there -- which is what the program
// below has to be able to read and name.
import Units

public func widen(_ m: Metric) -> Int32 { return m.doubled() }
public func origin() -> Int32 { return rawOf(21) }
public func rescaled(_ m: Metric, by n: Int32) -> Metric {
    return Metric(raw: m.raw * n)
}
