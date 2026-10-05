// §3.3 Existential Any Types and Protocol Type Erasure

protocol Renderer<Canvas> {
    associatedtype Canvas
    associatedtype Element
    func render(_ elem: Element, into canvas: inout Canvas)
}

struct PixelCanvas {}

struct TextRenderer: Renderer {
    typealias Canvas = PixelCanvas
    typealias Element = String

    func render(_ elem: String, into canvas: inout PixelCanvas) {
        print("Rendering: \(elem)")
    }
}

struct AnyRenderer<Canvas, Element>: Renderer {
    private let _render: (Element, inout Canvas) -> Void

    init<R: Renderer>(_ renderer: R) where R.Canvas == Canvas, R.Element == Element {
        self._render = renderer.render
    }

    func render(_ elem: Element, into canvas: inout Canvas) {
        _render(elem, &canvas)
    }
}

func invokeRenderer(r: any Renderer<PixelCanvas>, elem: String, canvas: inout PixelCanvas) {
    _ = (r, elem, canvas)
}
