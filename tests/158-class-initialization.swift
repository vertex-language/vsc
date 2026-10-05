// Two-phase initialization: own properties first, then super, then the rest.
class Base {
    var log: [String] = []
    init() {
        log.append("base init")
        setup()
    }
    func setup() { log.append("base setup") }
}
class Derived: Base {
    let factor: Int
    init(factor: Int) {
        self.factor = factor
        super.init()
        log.append("derived init with \(self.factor)")
    }
    override func setup() { log.append("derived setup sees \(factor)") }
}
print(Derived(factor: 7).log)
