// while tests before each pass, so it can run zero times.
var n = 0
var total = 0
while n < 10 {
    total += n
    n += 1
}
print(total, n)
while n < 0 {
    print("never")
}
