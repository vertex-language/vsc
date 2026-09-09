// vil: twice
final class Box { var n: Int = 0 }
func twice(_ b: Box) -> Box {
    let first = b
    let second = first
    return second
}
