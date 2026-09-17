// A type may be its own iterator, conforming to Sequence and
// IteratorProtocol with a mutating next() whose Element is a
// Result<Int, PortError>?. An optional payload enum is the enum's words
// and a tag byte after them; a case returned where the optional is
// declared is wrapped; and the witness a conformance calls is handed the
// conformer's storage to change.
enum PortError: Error {
    case refused(Int)
    case closed
}

struct Ports: Sequence, IteratorProtocol {
    var next_: Int
    let last: Int

    mutating func next() -> Result<Int, PortError>? {
        if next_ > last {
            return nil
        }
        let p = next_
        next_ += 1
        if p % 3 == 0 {
            return .failure(.refused(p))
        }
        if p == 5 {
            return .failure(.closed)
        }
        return .success(p * 10)
    }
}

func main() -> Int32 {
    var ports = Ports(next_: 1, last: 7)
    var total = 0
    while let r = ports.next() {
        switch r {
        case .success(let v):
            total += v
        case .failure(let e):
            switch e {
            case .refused(let q):
                total += q
            case .closed:
                total += 1000
            }
        }
    }
    return Int32(total % 251)
}
