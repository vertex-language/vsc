// §7 / §4.6 Result Builders

protocol HTMLComponent {
    func render() -> String
}

struct Text: HTMLComponent {
    let content: String
    func render() -> String { content }
}

struct Empty: HTMLComponent {
    func render() -> String { "" }
}

@resultBuilder
struct HTMLBuilder {
    static func buildBlock(_ components: HTMLComponent...) -> HTMLComponent {
        struct Combined: HTMLComponent {
            let items: [HTMLComponent]
            func render() -> String {
                items.map { $0.render() }.joined()
            }
        }
        return Combined(items: components)
    }

    static func buildOptional(_ component: HTMLComponent?) -> HTMLComponent {
        return component ?? Empty()
    }

    static func buildEither(first component: HTMLComponent) -> HTMLComponent {
        return component
    }

    static func buildEither(second component: HTMLComponent) -> HTMLComponent {
        return component
    }

    static func buildArray(_ components: [HTMLComponent]) -> HTMLComponent {
        struct Group: HTMLComponent {
            let list: [HTMLComponent]
            func render() -> String { list.map { $0.render() }.joined() }
        }
        return Group(list: components)
    }

    static func buildExpression(_ text: String) -> HTMLComponent {
        return Text(content: text)
    }
}

func buildPage(@HTMLBuilder _ content: () -> HTMLComponent) -> HTMLComponent {
    return content()
}

func testBuilderUsage(flag: Bool, items: [String]) -> HTMLComponent {
    return buildPage {
        "Header"
        if flag {
            Text(content: "True Branch")
        } else {
            Text(content: "False Branch")
        }
        for item in items {
            Text(content: item)
        }
    }
}
