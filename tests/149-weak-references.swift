// A weak reference does not keep its object alive and becomes nil.
class Person {
    let name: String
    var apartment: Apartment?
    init(_ n: String) { name = n }
    deinit { print("person gone") }
}
class Apartment {
    weak var tenant: Person?
    deinit { print("apartment gone") }
}
var p: Person? = Person("ann")
var apt: Apartment? = Apartment()
p!.apartment = apt
apt!.tenant = p
print(apt!.tenant?.name as Any)
p = nil
print(apt!.tenant as Any)
apt = nil
