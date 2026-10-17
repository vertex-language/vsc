// map over an array of structs reading a field through a generic max.
struct Interface {
    let Name: String
    let Location: Int
}
struct Inputs { let list: [Interface] }
let vertex = Inputs(list: [Interface(Name: "pos", Location: 2), Interface(Name: "uv", Location: -1)])
let locations = vertex.list.map { max(0, $0.Location) }
print(locations, vertex.list.map { $0.Name })
