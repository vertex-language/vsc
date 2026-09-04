func use() throws -> Int {
    do {
        return try inner()
    } catch {
        return 0
    }
}
func inner() throws -> Int { return 1 }
