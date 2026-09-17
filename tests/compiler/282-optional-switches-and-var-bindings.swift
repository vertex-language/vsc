// A switch over an optional names Optional's own cases -- `.some(let x)`,
// `let x?`, `.some`, `.none`, `nil`, `_` -- and a binding condition may
// bind a variable the body changes.

func present(_ o: Int?) -> Int {
    switch o {
    case .some: return 1
    case .none: return 0
    }
}

func label(_ o: String?) -> String {
    switch o {
    case .some(let s): return "some " + s
    case nil: return "nil"
    }
}

func question(_ o: String?) -> String {
    switch o {
    case let s?: return s + "?"
    default: return "default"
    }
}

func either(_ o: String?) -> Int {
    switch o {
    case .none: return 0
    case .some: return 2
    }
}

func anything(_ o: [Int]?) -> Int {
    switch o {
    case _: return 7
    }
}

func largest(_ xs: [Int]) -> Int {
    guard var best = xs.first else { return -1 }
    for x in xs where x > best { best = x }
    if var last = xs.last {
        last += 100
        best += last
    }
    return best
}

func joined(_ xs: [String]) -> String {
    guard var s = xs.first else { return "" }
    s += "!"
    if var t = xs.last {
        t += "?"
        s += t
    }
    return s
}

func check(_ ok: Bool, _ n: Int32) -> Int32 { return ok ? 0 : n }

func main() -> Int32 {
    var failed: Int32 = 0
    failed += check(present(3) == 1 && present(nil) == 0, 1)
    failed += check(label("x") == "some x" && label(nil) == "nil", 2)
    failed += check(question("y") == "y?" && question(nil) == "default", 3)
    failed += check(either("z") == 2 && either(nil) == 0 && anything(nil) == 7, 4)
    failed += check(largest([3, 9, 2]) == 111 && largest([]) == -1, 5)
    failed += check(joined(["a", "b"]) == "a!b?" && joined([]) == "", 6)
    print(label("x"), question(nil), largest([3, 9, 2]), joined(["a", "b"]))
    return failed
}
