// The edges where integer operations are defined but surprising.
func edges(_ minusOne: Int, _ zero: Int) {
    print(Int.min / 1, Int.min % 1, -Int.max - 1 == Int.min)
    print(Int8.min.magnitude, Int8.min &* Int8(minusOne), UInt.max &+ 1)
    print(Int.min.dividedReportingOverflow(by: minusOne), 5.remainderReportingOverflow(dividingBy: zero))
    print((-8) >> 1, (-1) >> 63, (-1 as Int8) << 7)
}
edges(-1, 0)
