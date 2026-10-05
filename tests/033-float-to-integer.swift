// Floating to integer truncates toward zero; rounding is asked for by name.
for d in [2.7, -2.7, 2.5, -2.5, 0.49] {
    print(Int(d), d.rounded(), d.rounded(.down), d.rounded(.up), d.rounded(.towardZero))
}
