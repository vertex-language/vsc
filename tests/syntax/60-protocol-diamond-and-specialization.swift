// §6.6 Diamond Protocol Inheritance and Conditional Specialization

protocol Node {
    var identifier: String { get }
}

protocol VisualNode: Node {
    var bounds: (x: Double, y: Double, width: Double, height: Double) { get }
}

protocol InteractiveNode: Node {
    func handleEvent(_ event: String) -> Bool
}

protocol ControlNode: VisualNode, InteractiveNode {
    var isEnabled: Bool { get set }
}

class BaseComponent {
    var tag: Int = 0
}

extension ControlNode where Self: BaseComponent {
    func reset() {
        self.tag = 0
        self.isEnabled = true
    }
}

class Button: BaseComponent, ControlNode {
    var identifier: String = "btn_1"
    var bounds = (x: 0.0, y: 0.0, width: 100.0, height: 40.0)
    var isEnabled: Bool = true

    func handleEvent(_ event: String) -> Bool {
        return isEnabled && event == "click"
    }
}

func inspectControl(ctrl: some ControlNode) {
    print("Node \(ctrl.identifier): enabled=\(ctrl.isEnabled)")
}
