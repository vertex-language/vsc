// An enum's computed and static members. They were never recorded:
// readMembers took a var declaration only where the type had somewhere
// to put all three kinds, and an enum has no stored properties and so
// no field sink -- which dropped its computed and static ones with
// them, and `Dir.count` was "no member 'count'" for something the
// type plainly declares.
enum Dir {
    case up
    case down

    static var count: Int32 { return 2 }

    var code: Int32 {
        switch self {
        case .up: return 1
        case .down: return 2
        }
    }

    func flipped() -> Dir {
        switch self {
        case .up: return .down
        case .down: return .up
        }
    }
}

func main() -> Int32 {
    let d = Dir.up
    return Dir.count * 10 + d.code + d.flipped().code
}
