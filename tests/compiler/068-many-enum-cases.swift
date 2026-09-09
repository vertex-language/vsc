// More cases than a byte would hold, so the tag is not a byte.
enum Big {
    case c0, c1, c2, c3, c4, c5, c6, c7, c8, c9
    case c10, c11, c12, c13, c14, c15, c16, c17, c18, c19
}

func value(_ b: Big) -> Int32 {
    switch b {
    case .c0: return 0
    case .c1: return 1
    case .c2: return 2
    case .c3: return 3
    case .c4: return 4
    case .c5: return 5
    case .c6: return 6
    case .c7: return 7
    case .c8: return 8
    case .c9: return 9
    case .c10: return 10
    case .c11: return 11
    case .c12: return 12
    case .c13: return 13
    case .c14: return 14
    case .c15: return 15
    case .c16: return 16
    case .c17: return 17
    case .c18: return 18
    case .c19: return 19
    }
}

func main() -> Int32 {
    return value(.c19) + value(.c0) + value(.c9)
}
