// Compiled by this compiler, linked against the library above.
import Failing

func main() -> Int32 {
    // The path that does not fail.
    if let a = try? mustBePositive(42) {
        if a != 42 { return 91 }
    } else {
        return 92
    }

    // The path that does.
    if let b = try? mustBePositive(-1) { return 93 + b }

    if let c = try? halved(84) {
        if c != 42 { return 94 }
    } else {
        return 95
    }
    if let bad = try? halved(7) { return 96 + bad }

    if let d = try? between(42, 0, 100) {
        if d != 42 { return 97 }
    } else {
        return 98
    }
    if let e = try? between(-1, 0, 100) { return 99 + e }

    // Called twice in a row: the register has to be cleared before
    // each, not once.
    if let one = try? always(1) {
        if one != 1 { return 100 }
    } else {
        return 101
    }
    if let two = try? always(2) {
        if two != 2 { return 102 }
    } else {
        return 103
    }

    if let f = try? mustBePositive(42) { return f }
    return 104
}
