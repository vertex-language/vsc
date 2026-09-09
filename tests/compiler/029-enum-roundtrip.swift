// An enum passed and returned keeps its case.
enum Direction {
    case north
    case south
    case east
    case west
}

func opposite(_ d: Direction) -> Direction {
    switch d {
    case .north: return .south
    case .south: return .north
    case .east: return .west
    case .west: return .east
    }
}

func number(_ d: Direction) -> Int32 {
    switch d {
    case .north: return 1
    case .south: return 2
    case .east: return 3
    case .west: return 4
    }
}

func main() -> Int32 {
    return number(opposite(.north)) * 10 + number(opposite(.east))
}
