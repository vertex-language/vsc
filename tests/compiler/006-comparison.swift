// Each ordering, read as a Bool.
func main() -> Int32 {
    let a: Int32 = 3
    let b: Int32 = 9
    var score: Int32 = 0
    if a < b { score = score + 1 }
    if a <= b { score = score + 2 }
    if b > a { score = score + 4 }
    if b >= a { score = score + 8 }
    if a == 3 { score = score + 16 }
    if a != b { score = score + 32 }
    return score
}
