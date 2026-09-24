// init? returns nil when it cannot make a value.
struct Even {
    let value: Int
    init?(_ n: Int) {
        guard n % 2 == 0 else { return nil }
        value = n
    }
}
for n in [4, 5] {
    if let e = Even(n) { print("even", e.value) } else { print(n, "refused") }
}
print(Even(10)?.value as Any, Even(3)?.value as Any)
