// for-in over half-open and closed ranges, and an empty one.
var s = ""
for i in 0..<5 { s += String(i) }
print(s)
s = ""
for i in 1...5 { s += String(i) }
print(s)
for _ in 5..<5 { print("never") }
for i in (1...4).reversed() { print(i, terminator: " ") }
print()
