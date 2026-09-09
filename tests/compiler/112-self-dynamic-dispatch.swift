// A method calling one of its own that a subclass overrode: the dispatch is on the object, not on where the call was written.
class Base {
  func step() -> Int32 { return 1 }
  func twice() -> Int32 { return step() + step() }
}
class Sub: Base { override func step() -> Int32 { return 21 } }
func main() -> Int32 { let s: Base = Sub(); return s.twice() }
