// §6.7 Failable and Convenience Initializers with Inheritance and Deinit

class BaseResource {
    let identifier: String
    var isOpen: Bool

    init?(identifier: String) {
        guard !identifier.isEmpty else { return nil }
        self.identifier = identifier
        self.isOpen = true
    }

    convenience init?(prefix: String, id: Int) {
        guard id >= 0 else { return nil }
        self.init(identifier: "\(prefix)_\(id)")
    }

    deinit {
        if isOpen {
            print("Resource \(identifier) was not properly closed")
        }
    }
}

class ManagedFile: BaseResource {
    let path: String

    // Overriding failable init with non-failable init
    override init(identifier: String) {
        self.path = "/tmp/\(identifier)"
        super.init(identifier: identifier.isEmpty ? "default" : identifier)!
    }

    convenience init?(strictPath: String) {
        guard strictPath.hasPrefix("/") else { return nil }
        self.init(identifier: strictPath.replacingOccurrences(of: "/", with: "_"))
    }
}

func testResourceLifecycle() {
    let r1 = BaseResource(identifier: "res_1")
    let r2 = BaseResource(prefix: "item", id: 42)
    let f1 = ManagedFile(identifier: "my_file")
    let f2 = ManagedFile(strictPath: "/var/log/app.log")
    _ = (r1, r2, f1, f2)
}
