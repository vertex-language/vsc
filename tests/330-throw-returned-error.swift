// A function declared `-> Error` returns the existential, and throwing
// what it returns throws the value inside, which a typed catch finds.
struct Boom: Error { let code: Int }
func wrap(_ e: Error) -> Error { return e }
func run() throws { throw wrap(Boom(code: 7)) }
do { try run() } catch let b as Boom { print("caught", b.code) } catch { print("other") }
