// A builtin value as `any Hashable`.
func hashes(_ h: any Hashable) -> Int {
    var a = Hasher()
    h.hash(into: &a)
    return 1
}
let h: any Hashable = 3
let hs: [any Hashable] = [1, "two", 3.0, true]
print(hashes(h), hashes("x"), hs.count)
let e: any Equatable = 3
print(type(of: h), type(of: e), hs.map { "\($0)" })
let i = 7 as any Hashable
print(i)
