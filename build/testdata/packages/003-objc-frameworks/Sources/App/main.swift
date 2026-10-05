import Registry

registry_add("door", 2, 3)
registry_add("window", 4, 1.5)
registry_add("wall", 10, 3)
print("count", registry_count())
print("door", registry_area("door"))
print("window", registry_area("window"))
print("missing", registry_area("roof"))
print("union width", registry_union_width())
