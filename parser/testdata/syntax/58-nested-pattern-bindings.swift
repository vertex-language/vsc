// §8 Nested Pattern Bindings with Mixed Let and Var

enum Tree<T> {
    case leaf(T)
    indirect case branch(left: Tree<T>, right: Tree<T>)
}

struct Coordinate {
    var x: Int
    var y: Int
}

func testPatternBindings(tree: Tree<Int>, pair: (Int, (String, (Int, Int)))) {
    switch pair {
    case (let code, (var label, (let x, var y))):
        label += "!"
        y += 10
        print("code: \(code), label: \(label), x: \(x), y: \(y)")
    }

    switch tree {
    case .branch(left: .leaf(let a), right: .leaf(var b)):
        b *= 2
        print("leaves: \(a), \(b)")
    case .branch(left: let leftSub, right: _):
        _ = leftSub
    case .leaf(var val):
        val += 1
        _ = val
    }

    let optionalPoint: (Int, Int)? = (10, 20)
    guard case (let px, var py)? = optionalPoint else {
        return
    }
    py += 1
    _ = (px, py)
}
