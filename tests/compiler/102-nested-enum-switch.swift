// A switch on an enum inside a case of another.
enum Outer { case x; case y }
enum Inner { case p; case q }
func f(_ o: Outer, _ i: Inner) -> Int32 {
  switch o {
  case .x: switch i { case .p: return 1; case .q: return 2 }
  case .y: switch i { case .p: return 20; case .q: return 40 }
  }
}
func main() -> Int32 { return f(.y, .q) + f(.x, .q) }
