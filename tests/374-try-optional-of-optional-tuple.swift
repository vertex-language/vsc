// try? of a call returning an optional tuple, compared with nil: the
// optional is flattened, and the tuple is never taken apart.
func pair(_ ok: Bool) -> (Int, Int)? { return ok ? (1, 2) : nil }
func parse(_ ok: Bool) throws -> (Int, Int)? { return pair(ok) }
func fails() throws -> (Int, Int)? { throw CancellationError() }
struct CancellationError: Error {}
print((try? parse(true)) != nil, (try? parse(false)) != nil, (try? fails()) != nil)
if let (a, b) = try? parse(true) { print(a + b) }
