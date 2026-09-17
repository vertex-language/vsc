// §4.8 Dynamic Member Lookup with Keypaths and String Names

@dynamicMemberLookup
struct JSONRecord {
    var dictionary: [String: Any]

    subscript(dynamicMember member: String) -> Any? {
        get { dictionary[member] }
        set { dictionary[member] = newValue }
    }
}

@dynamicMemberLookup
struct Lens<Entity> {
    var entity: Entity

    subscript<Property>(dynamicMember keyPath: KeyPath<Entity, Property>) -> Property {
        return entity[keyPath: keyPath]
    }

    subscript<Property>(dynamicMember keyPath: WritableKeyPath<Entity, Property>) -> Property {
        get { entity[keyPath: keyPath] }
        set { entity[keyPath: keyPath] = newValue }
    }
}

struct Profile {
    var username: String
    var age: Int
}

func testDynamicMemberLookup() {
    var json = JSONRecord(dictionary: ["status": 200, "message": "OK"])
    json.status = 201
    _ = json.status

    let p = Profile(username: "alex", age: 30)
    var lens = Lens(entity: p)
    print(lens.username)
    lens.age = 31
}
