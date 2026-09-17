// Two arms, one taken.
func classify(_ n: Int32) -> Int32 {
    if n > 0 {
        return 1
    } else {
        return 2
    }
}

func main() -> Int32 {
    return classify(5) * 10 + classify(-5)
}
