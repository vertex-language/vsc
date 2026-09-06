// Take a number apart and put it back together.
func digitSum(_ n: Int32) -> Int32 {
    var rest = n
    var total: Int32 = 0
    while rest > 0 {
        total = total + rest % 10
        rest = rest / 10
    }
    return total
}

func reversed(_ n: Int32) -> Int32 {
    var rest = n
    var out: Int32 = 0
    while rest > 0 {
        out = out * 10 + rest % 10
        rest = rest / 10
    }
    return out
}

func main() -> Int32 {
    return digitSum(12345) + reversed(123) / 100
}
