// §4.8 KeyPath Stress: Self, Subscripts, Optional Chaining, and Inferred Roots

struct City {
    var name: String
    var population: Int
}

struct Country {
    var name: String
    var cities: [City]
    var capital: City?
    var matrix: [[Int]]
}

func testKeyPathStress() {
    // Root with .self
    let citySelfKP = \City.self
    let inferredSelfKP: KeyPath<City, City> = \.self

    // Inferred root with optional chaining and subscripts
    let firstCityName: KeyPath<Country, String?> = \.cities[0].name
    let capitalPop: KeyPath<Country, Int?> = \.capital?.population

    // Multi-dimensional subscript in keypath
    let matrixElem = \Country.matrix[0][1]

    // Dictionary subscript keypath
    let dictLookup = \[String: City].["capital"]?.population

    let paris = City(name: "Paris", population: 2_161_000)
    let france = Country(
        name: "France",
        cities: [paris],
        capital: paris,
        matrix: [[1, 2], [3, 4]]
    )

    let pop = france[keyPath: capitalPop]
    let cSelf = paris[keyPath: citySelfKP]
    _ = (citySelfKP, inferredSelfKP, firstCityName, matrixElem, dictLookup, pop, cSelf)
}
