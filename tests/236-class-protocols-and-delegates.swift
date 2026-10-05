// A class-only protocol, held weakly: the delegate pattern.
protocol DownloadDelegate: AnyObject {
    func progress(_ p: Int)
    func finished(_ name: String)
}
final class Downloader {
    weak var delegate: DownloadDelegate?
    func run(_ name: String) {
        for p in stride(from: 0, through: 100, by: 50) { delegate?.progress(p) }
        delegate?.finished(name)
    }
}
final class Screen: DownloadDelegate {
    var log: [String] = []
    func progress(_ p: Int) { log.append("\(p)%") }
    func finished(_ name: String) { log.append("done \(name)") }
    deinit { print("screen gone") }
}
let d = Downloader()
var s: Screen? = Screen()
d.delegate = s
d.run("file")
print(s!.log)
s = nil
d.run("again")
print(d.delegate == nil)
