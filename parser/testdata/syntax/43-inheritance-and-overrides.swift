// §6.5 Inheritance, Overrides, and Subclasses

open class Component {
    open var title: String {
        get { "Base Component" }
        set { }
    }

    public required init() { }

    open func layout() {
        print("base layout")
    }

    open subscript(index: Int) -> String {
        return "item \(index)"
    }
}

public class ContainerView: Component {
    private var _title: String = ""

    public required init() {
        super.init()
    }

    public override var title: String {
        get { _title.isEmpty ? super.title : _title }
        set { _title = newValue }
    }

    public override func layout() {
        super.layout()
        print("container layout")
    }

    public override subscript(index: Int) -> String {
        return "container: \(super[index])"
    }
}

public final class LeafView: ContainerView {
    public final override func layout() {
        super.layout()
        print("leaf layout")
    }
}
