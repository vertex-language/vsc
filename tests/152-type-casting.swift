// is tests a dynamic type; as? and as! cast down to it.
class Media { let title: String; init(_ t: String) { title = t } }
class Movie: Media { let director = "dir" }
class Song: Media { let artist = "art" }
let library: [Media] = [Movie("m1"), Song("s1"), Movie("m2")]
var movies = 0
for item in library {
    if item is Movie { movies += 1 }
    if let s = item as? Song { print("song", s.title, s.artist) }
    if let m = item as? Movie { print("movie", m.title, m.director) }
}
print(movies, (library[0] as! Movie).director)
