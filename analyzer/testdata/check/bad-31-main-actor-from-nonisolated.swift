// What @MainActor refuses: synchronous isolated code called, and
// isolated state read or written, from a synchronous context that is
// not on the main actor; and, from an async one, without `await`.
@MainActor
final class Model {
    var count = 0
    func bump() { count += 1 }
}

@MainActor func redraw() {}

func sync(_ m: Model) {
    redraw()
    m.bump()
    print(m.count)
}

func async(_ m: Model) async {
    redraw()
    m.count = 2
    print(m.count)
}
