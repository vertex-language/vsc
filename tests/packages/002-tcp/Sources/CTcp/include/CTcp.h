#pragma once

#ifdef __cplusplus
extern "C" {
#endif

// A socket listening on 127.0.0.1 at a port the system chose, or -1.
int tcp_listen_loopback(void);

// The port a socket is bound to, or -1.
int tcp_port(int fd);

// A socket connected to 127.0.0.1 at port, or -1.
int tcp_connect_loopback(int port);

// The next connection a listening socket has, or -1.
int tcp_accept(int fd);

// Sends a 32-bit integer in network byte order: 4 on success.
int tcp_send_int(int fd, int value);

// Receives one, or -1 where the peer closed or failed.
int tcp_receive_int(int fd, int fallback);

int tcp_close(int fd);

#ifdef __cplusplus
}
#endif
