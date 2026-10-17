// Dictionary.removeAll(), with and without keepingCapacity, then reused.
var d = ["a": 1]
d["b"] = 2
d.removeAll()
print(d.count, d.isEmpty)
d["c"] = 3
print(d)
var k = [1: "x", 2: "y"]
k.removeAll(keepingCapacity: true)
print(k.count, k[1] ?? "none")
