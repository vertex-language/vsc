// sorted(by:) of a large array finishes: 300,000 elements.
var x: UInt64 = 88172645463325252
var xs: [Int] = []
xs.reserveCapacity(300_000)
for _ in 0..<300_000 {
    x ^= x << 13; x ^= x >> 7; x ^= x << 17
    xs.append(Int(x % 1_000_000))
}
let s = xs.sorted(by: <)
var ok = true
for i in 1..<s.count where s[i - 1] > s[i] { ok = false }
print(ok, s.first!, s.last!, s[150_000])
