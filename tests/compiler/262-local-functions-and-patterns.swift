// Things an event loop is written with: a nested function that changes
// the variables around it, a case condition over an event that carries a
// String, a switch clause that matches several cases, a tuple taken apart
// by a declaration, and a closure whose result is thrown away.
enum Event {
    case text(String)
    case resized(Double, Double)
    case scaled(Double)
    case frame(Double)
    case other
}

var queue: [Event] = [.text("hi"), .resized(3, 4), .scaled(2), .other, .frame(1.5), .text("bye")]
var at = 0

func next() -> Event? {
    if at >= queue.count { return nil }
    at += 1
    return queue[at - 1]
}

func size() -> (width: Int, height: Int, label: String) {
    return (width: 640, height: 400, label: "window")
}

func main() -> Int32 {
    var waiting = false
    var requests = 0
    var redraws = 0

    // A nested function capturing two variables by reference.
    func changed() {
        if !waiting {
            requests += 1
            waiting = true
        }
    }

    var texts = 0
    var frameAt = 0.0
    while let e = next() {
        if case .text(let t) = e {
            texts += t.count
        }
        switch e {
        case .resized(_, _), .scaled(_):
            redraws += 1
            changed()
        case .frame(let t):
            frameAt = t
            waiting = false
        default:
            break
        }
    }
    changed()

    let (w, h, label) = size()
    var (x, _) = (7, "unused")
    x += 1

    var buffer = [UInt8](repeating: 0, count: 4)
    _ = buffer.withUnsafeMutableBufferPointer { bp in
        bp.count
    }
    buffer[0] = 1

    let total = texts * 1000 + redraws * 100 + requests * 10 + Int(frameAt * 2)
        + (w + h) / 104 + label.count + x + Int(buffer[0])
    return Int32(total % 251)
}
