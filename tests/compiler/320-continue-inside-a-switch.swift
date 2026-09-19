// `continue` in a switch's arm goes to the loop around the switch, and
// on its way lets go of what the arm, the switch and the loop body
// hold: the element bound by the for-in, the value switched on.

final class Node {
    var kind: Int
    var text: String
    init(_ k: Int, _ t: String) { kind = k; text = t }
}

enum Shape {
    case dot
    case line(Int)
    case label(String)
}

func count(_ nodes: [Node]) -> Int {
    var n = 0
    for node in nodes {
        switch node.kind {
        case 0:
            if node.text.isEmpty { continue }
            n += 1
        case 1:
            continue
        default:
            n += 10
        }
    }
    return n
}

func describe(_ shapes: [Shape]) -> String {
    var out = ""
    outer: for s in shapes {
        switch s {
        case .dot:
            continue
        case .line(let n):
            if n < 0 { continue outer }
            out += "line\(n) "
        case .label(let text):
            if text.isEmpty { continue }
            out += text + " "
        }
        out += "| "
    }
    return out
}

func main() -> Int32 {
    print(count([Node(0, ""), Node(0, "a"), Node(1, "b"), Node(2, "c")]))
    print(describe([.dot, .line(-1), .line(2), .label(""), .label("x"), .dot]))
    var total = 0
    var i = 0
    while i < 3 {
        let items = [Node(0, "p"), Node(1, "q")]
        for item in items {
            switch item.text {
            case "q": continue
            default: total += item.kind + 1
            }
        }
        i += 1
    }
    print(total)
    return 0
}
