// Integer multiplication, with a sign on either side.
for (a, b) in [(6, 7), (-6, 7), (6, -7), (-6, -7), (0, 99), (1 << 20, 1 << 20)] {
    print(a * b)
}
