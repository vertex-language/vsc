// An empty array or dictionary literal assigned to an optional of one.
var r: [UInt8]? = nil
print(r == nil)
r = []
print(r?.count ?? -1)
r?.append(7)
print(r!)
var m: [String: Int]? = nil
m = [:]
m?["a"] = 1
print(m!)
