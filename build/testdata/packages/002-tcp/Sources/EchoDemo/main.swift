import TCP

// A client and a server in one process: the connection waits in the
// listener's backlog until the server accepts it.
let listener = Listener()
print("listening", listener.socket.isOpen(), listener.port() > 0)

let client = connect(port: listener.port())
let server = listener.accept()
print("connected", client.isOpen(), server.isOpen())

var total: Int32 = 0
var i: Int32 = 1
while i <= 5 {
    _ = client.send(i * i)
    let got = server.receive()
    _ = server.send(got + 1)
    total += client.receive()
    i += 1
}
print("echoed total", total)

client.close()
server.close()
listener.socket.close()
