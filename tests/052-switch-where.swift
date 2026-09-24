// A where clause adds a condition to a case.
for p in [(1, 1), (2, -2), (3, 7)] {
    switch p {
    case let (x, y) where x == y: print(x, y, "on x == y")
    case let (x, y) where x == -y: print(x, y, "on x == -y")
    case let (x, y): print(x, y, "elsewhere")
    }
}
