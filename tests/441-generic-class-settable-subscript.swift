// A generic class's subscript with a getter and a setter, written to and
// read from, an optional element set to nil.
struct Entity: Hashable { let id: Int }
final class Store<T> {
    private var items: [Entity: T] = [:]
    subscript(_ e: Entity) -> T? {
        get { items[e] }
        set { items[e] = newValue }
    }
    var count: Int { items.count }
}
struct Place { let x: Int, y: Int }
let places = Store<Place>()
let a = Entity(id: 1), b = Entity(id: 2)
places[a] = Place(x: 3, y: 4)
places[b] = Place(x: 0, y: 1)
print(places[a]!.x, places[b]!.y, places.count)
places[a] = nil
print(places[a] == nil, places.count)
let names = Store<String>()
names[a] = "ann"
names[a]! += "e"
print(names[a]!)
