// Designated and convenience initializers, and delegation through them.
class Vehicle {
    let wheels: Int
    let name: String
    init(wheels: Int, name: String) {
        self.wheels = wheels
        self.name = name
    }
    convenience init(bike name: String) { self.init(wheels: 2, name: name) }
}
class Car: Vehicle {
    let doors: Int
    init(doors: Int) {
        self.doors = doors
        super.init(wheels: 4, name: "car")
    }
    convenience init() { self.init(doors: 4) }
}
let v = Vehicle(bike: "bmx")
let c = Car()
print(v.wheels, v.name, c.wheels, c.name, c.doors)
