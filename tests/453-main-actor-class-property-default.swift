// A @MainActor class's stored property whose default makes another
// @MainActor value: the default runs on the main actor, as the class does.
@MainActor final class Model {
    var count = 3
    init() {}
}
@MainActor public final class Controller {
    let model = Model()
    var models = [Model(), Model()]
    public init() {}
}
@MainActor func run() {
    let c = Controller()
    print(c.model.count + c.models.count)
}
await MainActor.run { run() }
