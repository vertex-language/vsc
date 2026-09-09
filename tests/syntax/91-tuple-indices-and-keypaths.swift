// Tuple element numbers, and the key paths that name them.

typealias Nested = ((Int, String), (Double, Bool))

func read(_ n: Nested) -> String {
    let first = n.0.0
    let label = n.0.1
    let flag = n.1.1
    let deep = ((1, (2, 3)), 4).0.1.0
    _ = first
    _ = flag
    _ = deep
    return label
}

func write(_ n: inout Nested) {
    n.0.0 = 1
    n.1.0 = 2.5
}

let pairPath = \(Int, String).1
let firstPath: KeyPath<(Int, String), Int> = \.0
let nestedPath: KeyPath<((Int, String), Int), String> = \.0.1

struct Row {
    var cells: [Int]
    var name: String
}

let namePath = \Row.name
let cellPath = \Row.cells[0]
let selfPath = \Row.self
let optionalPath = \Row?.?.name

func byKeyPath(_ rows: [Row]) -> [String] {
    return rows.map(\.name)
}

func byTupleKeyPath(_ pairs: [(Int, String)]) -> [String] {
    return pairs.map(\.1)
}
