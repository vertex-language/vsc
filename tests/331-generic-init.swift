// An initializer generic over its argument: init<S: BinaryInteger>(from:).
struct Meters {
    let value: Int
    init<S: BinaryInteger>(from s: S) { value = Int(s) * 100 }
}
print(Meters(from: 3).value, Meters(from: UInt8(2)).value, Meters(from: Int16(-1)).value)
