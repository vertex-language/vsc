// guard exits early, and what it binds stays in scope after it.
func half(_ n: Int) -> Int? {
    guard n % 2 == 0 else {
        print(n, "is odd")
        return nil
    }
    return n / 2
}
func use(_ n: Int) {
    guard let h = half(n) else { return }
    print(n, "halves to", h)
}
use(10)
use(7)
