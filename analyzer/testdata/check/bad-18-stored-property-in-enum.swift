// An enum's storage is its cases. Swift refuses a stored property
// there rather than ignoring it, and this used to ignore it: the
// declaration was dropped along with every other var an enum
// declared, computed and static ones included.
enum E {
    case a
    var x: Int32 = 5
}
