// A return part way through leaves the rest unrun.
func firstOver(_ limit: Int32) -> Int32 {
    for i in 0..<100 {
        if Int32(i) * Int32(i) > limit {
            return Int32(i)
        }
    }
    return -1
}

func main() -> Int32 {
    return firstOver(50)
}
