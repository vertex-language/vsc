// A name written with its argument labels, an attribute that takes no
// arguments, and an accessor block that opens on its own line.

import Foundation

func handle(_ value: Int) {}
func handle(_ value: Int, again: Bool) {}
func move(to x: Int, from y: Int) {}

// A compound name refers to one declaration among the overloads
// rather than calling it.
let one = handle(_:)
let two = handle(_:again:)
let three = move(to:from:)
let makeString = String.init(describing:)

class Target: NSObject {
    @objc func tapped(_ sender: Any) {}

    func register() {
        _ = #selector(tapped(_:))
        _ = #selector(Target.tapped(_:))
    }
}

// An attribute that takes no arguments does not swallow the
// parenthesis that opens a function type's parameters.
func sleep(completionHandler: @escaping() -> Void) {}
func send(_ work: @Sendable() -> Void) {}
func lazily(_ make: @autoclosure() -> Int) {}
func spaced(completionHandler: @escaping () -> Void) {}
func viaC(_ fn: @convention(c)() -> Void) {}

// The brace of an accessor block may sit on the line below.
struct Sizes {
    var count: Int
    {
        return 3
    }

    var doubled: Int
    {
        get { count * 2 }
        set { count = newValue / 2 }
    }

    var observed: Int = 0
    {
        didSet { }
    }

    subscript(i: Int) -> Int
    {
        return i
    }
}
