// An escaping closure parameter called from inside another closure.
func later(_ body: @escaping () -> Void) -> () -> Void { body }
func f(_ k: @escaping (Int?) -> Void) -> () -> Void {
    later { k(nil); k(7) }
}
let run = f { v in print(v.map(String.init) ?? "nil") }
run()
