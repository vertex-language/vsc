// removeLast(k) drops k elements in place: popping a large array four at
// a time is linear, as a stack's pops are.
var stack: [Int] = []
for i in 0..<200_000 { stack.append(i) }
var popped = 0
var sum = 0
while stack.count >= 4 {
    sum += stack[stack.count - 1]
    stack.removeLast(4)
    popped += 4
}
print(popped, stack.count, sum)
var small = [1, 2, 3, 4, 5]
small.removeLast(2)
print(small)
small.removeLast(0)
print(small)
