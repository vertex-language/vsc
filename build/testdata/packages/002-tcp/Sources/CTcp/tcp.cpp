#include "CTcp.h"

#include <arpa/inet.h>
#include <netinet/in.h>
#include <cstring>
#include <sys/socket.h>
#include <unistd.h>

static void loopback(struct sockaddr_in *addr, int port) {
  memset(addr, 0, sizeof *addr);
  addr->sin_family = AF_INET;
  addr->sin_port = htons((unsigned short)port);
  addr->sin_addr.s_addr = htonl(INADDR_LOOPBACK);
}

int tcp_listen_loopback(void) {
  int fd = socket(AF_INET, SOCK_STREAM, 0);
  if (fd < 0)
    return -1;
  struct sockaddr_in addr;
  loopback(&addr, 0);
  if (bind(fd, (struct sockaddr *)&addr, sizeof addr) < 0 || listen(fd, 16) < 0) {
    close(fd);
    return -1;
  }
  return fd;
}

int tcp_port(int fd) {
  struct sockaddr_in addr;
  socklen_t len = sizeof addr;
  if (getsockname(fd, (struct sockaddr *)&addr, &len) < 0)
    return -1;
  return ntohs(addr.sin_port);
}

int tcp_connect_loopback(int port) {
  int fd = socket(AF_INET, SOCK_STREAM, 0);
  if (fd < 0)
    return -1;
  struct sockaddr_in addr;
  loopback(&addr, port);
  if (connect(fd, (struct sockaddr *)&addr, sizeof addr) < 0) {
    close(fd);
    return -1;
  }
  return fd;
}

int tcp_accept(int fd) { return accept(fd, 0, 0); }

int tcp_send_int(int fd, int value) {
  uint32_t wire = htonl((uint32_t)value);
  return (int)send(fd, &wire, sizeof wire, 0);
}

int tcp_receive_int(int fd, int fallback) {
  uint32_t wire;
  size_t got = 0;
  while (got < sizeof wire) {
    ssize_t n = recv(fd, (char *)&wire + got, sizeof wire - got, 0);
    if (n <= 0)
      return fallback;
    got += (size_t)n;
  }
  return (int)ntohl(wire);
}

int tcp_close(int fd) { return close(fd); }
