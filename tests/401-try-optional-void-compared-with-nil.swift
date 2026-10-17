// try? on a call returning Void is a Void?, compared with nil.
struct Failed: Error {}
func flush(_ ok: Bool) throws { if !ok { throw Failed() } }
print((try? flush(true)) != nil, (try? flush(false)) != nil)
func save(_ ok: Bool) async throws { if !ok { throw Failed() } }
print((try? await save(true)) != nil, (try? await save(false)) == nil)
