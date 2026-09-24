// An enum with plain cases, matched by switch and compared with ==.
enum Direction {
    case north, south, east, west
    var opposite: Direction {
        switch self {
        case .north: return .south
        case .south: return .north
        case .east: return .west
        case .west: return .east
        }
    }
}
let d = Direction.east
print(d, d.opposite, d == .east, d.opposite == .east)
