// A class whose stored properties all have defaults -- an empty literal, a
// call -- and no initializer is made with C().
final class Mentions {
    var names: [String] = []
    init() {}
}
final class Features {
    var ids: Set<String> = []
    var counts: [String: Int] = [:]
    let all = Mentions()
    var limit = 8
}
let f = Features()
f.ids.insert("a")
f.all.names.append("x")
print(f.ids.count, f.counts.isEmpty, f.all.names, f.limit)
