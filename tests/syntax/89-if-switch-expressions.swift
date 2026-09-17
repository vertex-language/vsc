// if and switch as expressions (SE-0380).

enum Weight { case light, medium, heavy }

let flag = true

let simple = if flag { 1 } else { 2 }

let chained = if flag {
    "yes"
} else if !flag {
    "no"
} else {
    "maybe"
}

let matched = switch Weight.medium {
case .light: 1
case .medium: 2
case .heavy: 3
}

func classify(_ n: Int) -> String {
    return if n < 0 { "negative" } else if n == 0 { "zero" } else { "positive" }
}

func implicitReturn(_ n: Int) -> String {
    if n < 0 { "negative" } else { "not negative" }
}

func assigned(_ n: Int) -> Int {
    var out = 0
    out = if n > 0 { n } else { -n }
    return out
}

func nested(_ w: Weight) -> Int {
    let x = switch w {
    case .light: if flag { 0 } else { 1 }
    case .medium, .heavy: switch w {
        case .heavy: 3
        default: 2
        }
    }
    return x
}

func inArgument(_ n: Int) -> Int {
    return abs(if n > 0 { n } else { -n })
}

func withEffects() async throws -> Int {
    let a = try await value()
    var total = 0
    total += try scoreIfPositive(a) ? 1 : 0
    return total
}

func value() async throws -> Int { 1 }
func scoreIfPositive(_ n: Int) throws -> Bool { n > 0 }

// The statement forms are unchanged.
func statements(_ w: Weight) {
    if flag { print("on") } else { print("off") }
    switch w {
    case .light: break
    default: break
    }
}
