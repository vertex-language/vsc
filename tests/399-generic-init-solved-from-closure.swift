// A generic class's init called without naming T, T solved from the
// trailing closure's result.
enum Load<T> {
    case ready(T)
    case failed(String)
}
final class Resource<T> {
    let key: String
    let fetch: () -> Load<T>
    init(of key: String, _ fetch: @escaping () -> Load<T>) {
        self.key = key
        self.fetch = fetch
    }
}
let r = Resource(of: "n") { Load.ready(3) }
if case .ready(let v) = r.fetch() { print(r.key, v + 1) }
let s = Resource(of: "s") { () -> Load<String> in .failed("gone") }
if case .failed(let why) = s.fetch() { print(s.key, why) }
