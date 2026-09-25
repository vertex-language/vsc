import Codes

print(Kind(rawValue: 2) == .unknown, Kind(rawValue: 6) == .byte, Kind(rawValue: 4) == nil, Kind.control.rawValue)
print(Tag(rawValue: "b") == .bold, Tag(rawValue: "italic") == .italic, Tag(rawValue: "i") == nil, Tag.bold.rawValue)
print(Level(rawValue: 8) == .high, Level.low.rawValue)
print([1, 2, 3, 5, 6].map { Kind(rawValue: $0) != nil })
