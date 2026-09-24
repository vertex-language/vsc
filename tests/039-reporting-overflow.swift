// The overflow-reporting methods return the wrapped value and a flag.
let a = Int8.max
let (sum, over) = a.addingReportingOverflow(1)
print(sum, over)
let r = a.multipliedReportingOverflow(by: 2)
print(r.partialValue, r.overflow)
let s = UInt8(3).subtractingReportingOverflow(5)
print(s.partialValue, s.overflow)
let d = Int.max.multipliedFullWidth(by: 4)
print(d.high, d.low)
