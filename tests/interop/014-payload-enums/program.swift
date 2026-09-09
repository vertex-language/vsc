// Compiled by this compiler, linked against the library above.
import Shapes2

func main() -> Int32 {
    // Made here, read there.
    if area(.dot) != 0 { return 91 }
    if area(.line(7)) != 7 { return 92 }
    if area(.box(6, 7)) != 42 { return 93 }
    if area(Shape.box(2, 3)) != 6 { return 94 }
    if after(.line(1), 42) != 42 { return 95 }

    // Made there, read here: the tag byte is swiftc's and the switch
    // on it is this compiler's.
    switch makeLine(42) {
    case .line(let n): if n != 42 { return 96 }
    default: return 97
    }
    switch makeBox(6, 7) {
    case .box(let w, let h): if w * h != 42 { return 98 }
    default: return 99
    }
    switch makeLine(1) {
    case .dot: return 100
    case .box: return 101
    case .line: break
    }

    if valueOf(reading(9)) != 9 { return 102 }
    if valueOf(reading(-1)) != -1 { return 103 }

    return area(.box(6, 7))
}
