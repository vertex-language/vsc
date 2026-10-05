// repeat-while tests after each pass, so it always runs once.
var n = 100
repeat {
    print("ran with", n)
    n += 1
} while n < 3
var steps = 0
var x = 27
repeat {
    x = x % 2 == 0 ? x / 2 : 3 * x + 1
    steps += 1
} while x != 1
print(steps)
