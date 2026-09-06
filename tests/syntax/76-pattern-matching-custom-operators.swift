// §8 / §4.2 Custom Pattern Matching with ~= Operator Overloads

struct RegexPattern {
    let pattern: String
}

func ~=(pattern: RegexPattern, value: String) -> Bool {
    return value.contains(pattern.pattern)
}

struct SemanticVersion {
    let major: Int
    let minor: Int
    let patch: Int
}

func ~=(pattern: ClosedRange<Int>, version: SemanticVersion) -> Bool {
    return pattern.contains(version.major)
}

func matchVersionAndText(v: SemanticVersion, text: String) {
    switch v {
    case 1...2:
        print("Legacy version 1 or 2")
    case 3...5:
        print("Current version 3 to 5")
    default:
        print("Unknown version")
    }

    switch text {
    case RegexPattern(pattern: "error"):
        print("Contains error")
    case RegexPattern(pattern: "warning"):
        print("Contains warning")
    default:
        print("Info only")
    }

    if case 3...4 = v {
        print("Specifically 3 or 4")
    }
}
