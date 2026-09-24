// stride(from:to:by:) excludes the end, stride(from:through:by:) includes it.
for i in stride(from: 0, to: 10, by: 3) { print(i, terminator: " ") }
print()
for i in stride(from: 10, through: 0, by: -5) { print(i, terminator: " ") }
print()
for x in stride(from: 0.0, through: 1.0, by: 0.25) { print(x, terminator: " ") }
print()
