// A switch over values compares in order.
func name(_ n: Int32) -> Int32 {
    switch n {
    case 0: return 10
    case 1: return 20
    case 2: return 30
    default: return 99
    }
}

func main() -> Int32 {
    return name(1) + name(7) - name(0) - name(2)
}
