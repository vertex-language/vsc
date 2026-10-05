// Enum payloads that mix a struct with small scalars: a UInt8-sized enum,
// an Int32 and a Bool beside a struct.
struct Point { var x: Double; var y: Double }
enum Btn { case left, right }
enum Event {
    case down(Point, button: Btn, clicks: Int32)
    case wheel(delta: Point, precise: Bool)
}
let es: [Event] = [.down(Point(x: 1, y: 2), button: .right, clicks: 2),
                   .wheel(delta: Point(x: 0, y: -3), precise: true)]
for e in es {
    switch e {
    case .down(let p, let b, let c):
        if b == .right { print("right", p.x, c) }
        switch b { case .left: print("l"); case .right: print("r") }
    case .wheel(let d, let precise):
        if precise { print("precise", d.y) }
    }
}
