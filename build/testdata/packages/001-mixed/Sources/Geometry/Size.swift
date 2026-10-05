import CMath

public struct Size {
    public var width: Int32
    public var height: Int32

    public init(width: Int32, height: Int32) {
        self.width = width
        self.height = height
    }

    // The area, scaled by the C library.
    public func scaledArea() -> Int32 {
        return cmath_scale(width * height)
    }

    public func perimeter() -> Int32 {
        return cmath_add(width, height) * 2
    }
}
