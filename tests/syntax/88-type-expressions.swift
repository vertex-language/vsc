// Types written where an expression goes.

protocol Shape {}
struct Circle: Shape {}

// `any` and `some` are types and nothing else, so a spelling that
// starts with one is a type even in expression position.
let shapes = [any Shape]()
let table = [String: any Shape]()
let shapeMeta = (any Shape).self
let arrayMeta = [any Shape].self

// A type written with attributes: the attribute is what says this is
// a function type rather than a parenthesized expression.
func rebind(_ p: UnsafeRawPointer) {
    let fn = unsafeBitCast(p, to: (@convention(c) (Int) -> Int).self)
    _ = fn
}

// The ordinary spellings keep their expression shape.
let plainMeta = [Int].self
let optionalMeta = Int?.self
let tupleMeta = (Int, String).self
let dictMeta = [Int: String].self

// `_` is a type: the analyzer is being asked which one.
let inferred: Array<_> = [1, 2, 3]
let inferredElement: [_] = ["a"]

// An initializer named by its argument labels rather than called.
let makeString = String.init(describing:)
let makeRepeating = String.init(repeating:count:)
let describe = String.init

func use() {
    _ = shapes
    _ = table
    _ = shapeMeta
    _ = arrayMeta
    _ = plainMeta
    _ = optionalMeta
    _ = tupleMeta
    _ = dictMeta
    _ = inferred
    _ = inferredElement
    _ = makeString
    _ = makeRepeating
    _ = describe
}
