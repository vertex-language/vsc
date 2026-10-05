// if with and without else, on each side of the branch.
for n in [3, 8] {
    if n > 5 {
        print(n, "big")
    } else {
        print(n, "small")
    }
    if n % 2 == 0 {
        print(n, "even")
    }
}
