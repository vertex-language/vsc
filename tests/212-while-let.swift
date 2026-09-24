// while let loops until an optional runs out.
var stack = [1, 2, 3]
while let top = stack.popLast() { print("popped", top) }
var it = "abc".makeIterator()
while let c = it.next() { print(c, terminator: "") }
print()
var n = 50
func next(_ x: Int) -> Int? { x > 1 ? x / 3 : nil }
while let m = next(n) { print(m, terminator: " "); n = m }
print()
