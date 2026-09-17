// && and || do not evaluate their right side unless they must.
func main() -> Int32 {
    let a = true
    let b = false
    var score: Int32 = 0
    if a && !b { score = score + 1 }
    if b || a { score = score + 2 }
    if !(a && b) { score = score + 4 }
    return score
}
