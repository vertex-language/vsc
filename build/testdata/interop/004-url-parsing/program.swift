import Net

func main() -> Int32 {
    if portOf(0) != 8443 { return 91 }
    if portOf(1) != 0 { return 92 }
    if portOf(2) != -1 { return 93 }
    if pathComponentCount(0) != 3 { return 94 }
    if queryValue(0) != 7 { return 95 }
    if queryValue(1) != 35 { return 96 }
    if percentEncodes() != 1 { return 97 }
    return queryValue(1) + 7
}
