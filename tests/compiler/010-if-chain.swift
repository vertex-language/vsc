// A chain picks the first arm that holds.
func band(_ n: Int32) -> Int32 {
    if n < 10 {
        return 1
    } else if n < 100 {
        return 2
    } else if n < 1000 {
        return 3
    }
    return 4
}

func main() -> Int32 {
    return band(5) + band(50) * 10 + band(500) * 100
}
