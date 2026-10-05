// A struct holding a class shares the object when the struct is copied.
final class Log { var lines: [String] = [] }
struct Worker {
    var name: String
    let log: Log
    func work() { log.lines.append(name) }
}
let shared = Log()
var a = Worker(name: "a", log: shared)
var b = a
b.name = "b"
a.work(); b.work()
print(a.name, b.name, shared.lines, a.log === b.log)
