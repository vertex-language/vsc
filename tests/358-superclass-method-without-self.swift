// A subclass calls its superclass's method by bare name, without self.
class Object {
    var props: [String: Int] = [:]
    func ordinarySet(_ key: String, _ v: Int) -> Bool { props[key] = v; return true }
}
final class ArrayObject: Object {
    func push(_ v: Int) -> Bool {
        return ordinarySet("\(props.count)", v)
    }
}
let a = ArrayObject()
print(a.push(5), a.push(6), a.props.count, a.props["1"]!)
