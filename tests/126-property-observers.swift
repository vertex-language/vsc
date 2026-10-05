// willSet and didSet run around each assignment.
struct Thermostat {
    var target: Int = 20 {
        willSet { print("will change", target, "to", newValue) }
        didSet {
            if target > 30 { target = 30 }
            print("did change from", oldValue, "to", target)
        }
    }
}
var t = Thermostat()
t.target = 22
t.target = 40
print(t.target)
