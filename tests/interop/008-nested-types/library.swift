// Types declared inside types, which is how Swift's own libraries are
// shaped: String.Index, Data.Deallocator, Font.Weight. An interface
// names one with both names, and the symbol of anything that takes
// one says both as well.
public struct Chart {
    public var scale: Int32

    public struct Point {
        public var x: Int32
        public var y: Int32
        public init(x: Int32, y: Int32) { self.x = x; self.y = y }
        public func sum() -> Int32 { return x + y }
    }

    public enum Axis {
        case horizontal
        case vertical
    }

    public init(scale: Int32) { self.scale = scale }
}

public func plot(_ p: Chart.Point) -> Int32 { return p.sum() }
public func along(_ a: Chart.Axis) -> Int32 {
    switch a {
    case .horizontal: return 1
    case .vertical: return 2
    }
}
public func scaled(_ p: Chart.Point, by n: Int32) -> Chart.Point {
    return Chart.Point(x: p.x * n, y: p.y * n)
}
