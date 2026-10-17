// split(whereSeparator:), on spaces or tabs, empty pieces dropped or kept.
let line = "GL_APICALL  void\tglClear (GLbitfield mask);"
let parts = line.split(whereSeparator: { $0 == " " || $0 == "\t" })
print(parts.count, parts)
print("a,,b".split(omittingEmptySubsequences: false, whereSeparator: { $0 == "," }))
print([1, 0, 2, 3, 0, 4].split(whereSeparator: { $0 == 0 }).map { Array($0) })
