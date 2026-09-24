// deinit runs when the last reference goes away.
class Resource {
    let name: String
    init(_ name: String) {
        self.name = name
        print("open", name)
    }
    deinit { print("close", name) }
}
func scope() {
    let r = Resource("a")
    print("using", r.name)
}
scope()
var held: Resource? = Resource("b")
let other = held
held = nil
print("b still held")
_ = other
var list: [Resource] = [Resource("c"), Resource("d")]
list.removeFirst()
print("end")
