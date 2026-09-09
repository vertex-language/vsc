// Built by swiftc, called by this compiler.
//
// The library is written the way a Swift library is written -- String,
// Array, sorting, Foundation's number formatting -- and exports
// nothing but the scalars a caller can hold. What this case is about
// is not the arithmetic but the names: the interface beside it is
// written the way swiftc writes one, with every name qualified by the
// module it came from.
import Foundation

public func mean(_ a: Int32, _ b: Int32) -> Int32 {
    return (a + b) / 2
}

public func medianOf(_ a: Int32, _ b: Int32, _ c: Int32) -> Int32 {
    let sorted = [a, b, c].sorted()
    return sorted[1]
}

public func digitsInDescription(_ n: Int32) -> Int32 {
    let text = "\(n)"
    return Int32(text.filter { $0.isNumber }.count)
}

public func roundedTenths(_ n: Int32) -> Int32 {
    let scaled = (Double(n) / 10.0).rounded()
    return Int32(scaled)
}
