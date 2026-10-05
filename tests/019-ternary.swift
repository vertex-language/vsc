// The conditional operator, nested and as a value.
for n in [-5, 0, 5] {
    let sign = n < 0 ? "negative" : n == 0 ? "zero" : "positive"
    print(n, sign)
}
let m = 3 > 2 ? 10 : 20
print(m * 2)
