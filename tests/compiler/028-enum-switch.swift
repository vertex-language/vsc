// An enum branches on its tag, not on a comparison.
enum Colour {
    case red
    case green
    case blue
}

func value(_ c: Colour) -> Int32 {
    switch c {
    case .red: return 1
    case .green: return 2
    case .blue: return 4
    }
}

func main() -> Int32 {
    return value(.red) + value(.green) * 10 + value(.blue) * 100
}
