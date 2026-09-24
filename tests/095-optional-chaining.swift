// ?. stops at the first nil and the whole chain becomes nil.
struct Engine { var power: Int }
struct Car { var engine: Engine? }
struct Owner { var car: Car? }
let owners = [Owner(car: Car(engine: Engine(power: 150))), Owner(car: Car(engine: nil)), Owner(car: nil)]
for o in owners {
    print(o.car?.engine?.power as Any, o.car?.engine?.power.description ?? "none")
}
