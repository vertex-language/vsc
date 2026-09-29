// An enum declared here, whose payload holds a class reference, thrown and caught.
final class Value { let n: Int; init(_ n: Int) { self.n = n } }
enum Completion: Error { case thrown(Value), aborted }
func run(_ n: Int) throws -> Int {
    if n > 2 { throw Completion.thrown(Value(n)) }
    return n
}
for i in [1, 5] {
    do { print(try run(i)) }
    catch Completion.thrown(let v) { print("thrown", v.n) }
    catch { print("other") }
}
