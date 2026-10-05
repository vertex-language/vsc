// An empty dictionary literal assigned into a dictionary of dictionaries.
var tables: [Int: [String: Double]] = [:]
tables[1] = [:]
tables[1]!["x"] = 1.5
tables[2] = [:]
print(tables.count, tables[1]!["x"]!, tables[2]!.isEmpty)
