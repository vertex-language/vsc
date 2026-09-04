// vil: rebinds
final class Box { var n: Int = 0 }
func rebinds(_ b: __owned Box) -> Box {
    let kept = b
    return kept
}
