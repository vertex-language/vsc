// §6.6 Protocols with Primary Associated Types and Complex Constraints

protocol Repository<Model> {
    associatedtype Model: Identifiable
    associatedtype ID = Model.ID
    associatedtype ErrorType: Error = Error

    func find(by id: ID) async throws(ErrorType) -> Model?
    func save(_ model: Model) async throws(ErrorType)
}

protocol Observable: AnyObject {
    func notifyObservers()
}

protocol ServiceLocator {
    static var shared: Self { get }
    func register<T>(_ service: T)
    func resolve<T>() -> T?
}

struct User: Identifiable {
    let id: Int
    var name: String
}

class InMemoryRepo: Repository, Observable {
    typealias Model = User
    typealias ErrorType = Never

    private var storage = [Int: User]()

    func find(by id: Int) async throws(Never) -> User? {
        return storage[id]
    }

    func save(_ model: User) async throws(Never) {
        storage[model.id] = model
        notifyObservers()
    }

    func notifyObservers() {
        print("Updated")
    }
}

func fetchUser(from repo: some Repository<User>) async -> User? {
    return try? await repo.find(by: 1)
}

func storeUsers(repo: any Repository<User>, users: [User]) async {
    for user in users {
        try? await repo.save(user)
    }
}
