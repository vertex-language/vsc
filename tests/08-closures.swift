// §4.6 Closures


// ClosureExpression with full signature
let multiply: (Int, Int) -> Int = { (a: Int, b: Int) -> Int in
    return a * b
}

// ClosureParameterClause: IdentifierList (no parens)
let addShort: (Int, Int) -> Int = { a, b in a + b }

// CaptureList
var counter = 0
let incrementer: () -> Void = { [counter] in
    print("captured counter: \(counter)")
}

// CaptureSpecifier: weak / unowned
class Node {
    var value = 0
    lazy var printValue: () -> Void = { [weak self] in
        print(self?.value ?? -1)
    }
    lazy var printUnowned: () -> Void = { [unowned self] in
        print(self.value)
    }
}

// async / throws in ClosureSignature
let asyncClosure: () async -> Int = { 42 }
let throwingClosure: () throws -> Int = { throw SampleErr.oops }
enum SampleErr: Error { case oops }

// TrailingClosures with LabeledTrailingClosure
func animate(duration: Double, animations: () -> Void, completion: (Bool) -> Void) {
    animations()
    completion(true)
}
animate(duration: 0.3) {
    print("animating")
} completion: { finished in
    print("done: \(finished)")
}