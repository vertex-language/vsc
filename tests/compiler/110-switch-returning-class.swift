// A switch that writes through a reference before returning it.
final class B { var n: Int32 = 0 }
enum Which { case one; case two }
func get(_ w: Which) -> B { let b = B(); switch w { case .one: b.n = 20; case .two: b.n = 22 }; return b }
func main() -> Int32 { return get(.one).n + get(.two).n }
