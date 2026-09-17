// A main that throws runs until an error gets out of it, and an error that
// does ends the program the way Swift ends one raised at top level: what
// was printed stays printed, and the process traps.

enum Failure: Error {
    case refused(String)
}

final class Resource {
    var name: String
    init(_ name: String) { self.name = name }
}

func open(_ name: String) throws -> Resource {
    if name.isEmpty {
        throw Failure.refused("no name")
    }
    return Resource(name)
}

func main() throws -> Int32 {
    let r = try open("config")
    print("opened", r.name)
    do {
        _ = try open("")
    } catch {
        print("caught one")
    }
    _ = try open("")
    print("not reached")
    return 0
}
