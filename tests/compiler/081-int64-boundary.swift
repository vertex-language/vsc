// Arithmetic at the top of the range, without crossing it.
func opaque(_ n: Int64) -> Int64 { return n }

func main() -> Int32 {
    let big = opaque(9223372036854775806)
    let sum = big + opaque(1)
    var score: Int32 = 0
    if sum == 9223372036854775807 { score = score + 1 }
    if sum > big { score = score + 2 }
    let low = opaque(-9223372036854775807)
    if low - opaque(1) == -9223372036854775808 { score = score + 4 }
    return score
}
