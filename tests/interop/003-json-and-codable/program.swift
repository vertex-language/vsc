import Json

func main() -> Int32 {
    if serializationRoundTrip() != 42 { return 91 }
    if codableRoundTrip(7) != 7 { return 92 }
    if parseFailureIsReported() != 1 { return 93 }
    return codableRoundTrip(42)
}
