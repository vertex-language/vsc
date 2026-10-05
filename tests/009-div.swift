// Signed division truncates toward zero.
for (a, b) in [(7, 2), (-7, 2), (7, -2), (-7, -2), (0, 5), (100, 100)] {
    print(a / b)
}
