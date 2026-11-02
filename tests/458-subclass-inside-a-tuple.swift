// A subclass in a tuple converts to the tuple of its superclass, labelled or
// not, as an argument, a variable's value, a return and an element.
class Event { func name() -> String { return "event" } }
class TransitionEvent: Event { override func name() -> String { return "transition" } }
final class Node { let id: Int; init(id: Int) { self.id = id } }
var events: [(node: Node, event: Event)] = []
let n = Node(id: 1)
events.append((node: n, event: TransitionEvent()))
let pair: (Int, Event) = (2, TransitionEvent())
func one() -> (Int, Event) { return (3, TransitionEvent()) }
func take(_ t: (Event, Int)) -> String { return t.0.name() + String(t.1) }
let list: [(Int, Event)] = [(4, TransitionEvent()), (5, Event())]
print(events[0].node.id, events[0].event.name(), pair.1.name(), one().1.name(), take((TransitionEvent(), 6)))
print(list.map { $0.1.name() })
let kept = TransitionEvent()
let again: (Int, Event) = (7, kept)
print(again.1 === kept)
