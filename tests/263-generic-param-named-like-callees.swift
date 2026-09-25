// A generic function whose parameter is named T, calling generic methods
// whose own parameters are named T too: map<T>, reduce, compactMap. The
// two T's are different parameters.
protocol Keyed { static func key(_ x: Self) -> UInt32 }
extension Int32: Keyed { static func key(_ x: Int32) -> UInt32 { UInt32(bitPattern: x) } }
extension UInt8: Keyed { static func key(_ x: UInt8) -> UInt32 { UInt32(x) } }

func keys<T: Keyed>(_ xs: [T]) -> [UInt64] {
    return xs.map { UInt64(T.key($0)) }
}

func total<T: Keyed>(_ xs: [T]) -> UInt64 {
    return xs.map { T.key($0) }.reduce(UInt64(0)) { $0 + UInt64($1) }
}

func evens<T: Keyed>(_ xs: [T]) -> [String] {
    return xs.compactMap { T.key($0) % 2 == 0 ? "\(T.key($0))" : nil }
}

print(keys([Int32(1), -1, 7]))
print(keys([UInt8(3), 200]))
print(total([Int32(5), 6]))
print(evens([UInt8(2), 3, 4]))
