// Compiled by this compiler, linked against the library above.
import Chrono

func main() -> Int32 {
    if epochYearUTC() != 1970 { return 91 }
    if daysBetweenUTC(0, 10) != 10 { return 92 }
    if daysBetweenUTC(10, 0) != -10 { return 93 }
    if uuidIsStable() != 1 { return 94 }
    return 42
}
