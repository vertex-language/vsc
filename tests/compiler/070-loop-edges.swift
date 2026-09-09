// Ranges that run once, and ranges that do not run at all.
func main() -> Int32 {
    var once: Int32 = 0
    for _ in 0..<1 { once = once + 1 }

    var never: Int32 = 0
    for _ in 5..<5 { never = never + 1 }

    var closedOnce: Int32 = 0
    for _ in 3...3 { closedOnce = closedOnce + 1 }

    var whileNever: Int32 = 0
    while false { whileNever = whileNever + 1 }

    var repeatOnce: Int32 = 0
    repeat { repeatOnce = repeatOnce + 1 } while false

    return once * 10000 + never * 1000 + closedOnce * 100 + whileNever * 10 + repeatOnce
}
