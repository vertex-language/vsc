// Code written once over BinaryInteger and FixedWidthInteger, at many widths.
func digitSum<T: BinaryInteger>(_ n: T) -> T {
    var n = n < 0 ? 0 - n : n
    var s: T = 0
    while n > 0 { s += n % 10; n /= 10 }
    return s
}
func describeWidth<T: FixedWidthInteger>(_: T.Type) -> String {
    "\(T.bitWidth) bits, \(T.isSigned ? "signed" : "unsigned"), max \(T.max)"
}
print(digitSum(12345), digitSum(Int8(-99)), digitSum(UInt64.max))
print(describeWidth(Int8.self))
print(describeWidth(UInt32.self))
