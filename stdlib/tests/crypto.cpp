// The runtime's cryptographic routines (crypto.cpp), which a program calls
// where its processor lacks the instruction. Each must answer what the
// instruction does: against outputs an Apple M1's own instructions gave
// (crypto_golden.h), and, for what that core does not have or that is
// x86-64's shape, against the standards' own vectors. A failure is a line
// of output; build/runtime_test.go wants none.
#include "runtime.h"
#include "platform.h"

using namespace vertex;

#include "crypto_golden.h"

static bool same(const char* a, const char* b) {
  while (*a != 0 && *a == *b)
    a++, b++;
  return *a == *b;
}

static void say(const char* what) {
  usize n = 0;
  while (what[n] != 0)
    n++;
  vertex_pal_write(1, reinterpret_cast<const u8*>(what), n);
  vertex_pal_write(1, reinterpret_cast<const u8*>("\n"), 1);
}

static bool equal16(const u8* a, const char* b) {
  for (int i = 0; i < 16; i++)
    if (a[i] != u8(b[i]))
      return false;
  return true;
}

static bool equal16(const u8* a, const u8* b) { return equal16(a, reinterpret_cast<const char*>(b)); }

int main() {
  for (const Golden& g : golden) {
    const u8* a = reinterpret_cast<const u8*>(g.a);
    const u8* b = reinterpret_cast<const u8*>(g.b);
    const u8* c = reinterpret_cast<const u8*>(g.c);
    u8 out[16];
    const char* v = g.verb;
    if (same(v, "aes_enc")) vertex_aes_enc(out, a, b);
    else if (same(v, "aes_enc_last")) vertex_aes_enc_last(out, a, b);
    else if (same(v, "aes_dec")) vertex_aes_dec(out, a, b);
    else if (same(v, "aes_dec_last")) vertex_aes_dec_last(out, a, b);
    else if (same(v, "aes_inv_mix_columns")) vertex_aes_inv_mix_columns(out, a);
    else if (same(v, "i64x2_clmul")) vertex_i64x2_clmul(out, a, b, g.lit);
    else if (same(v, "sha1_rounds_ch")) vertex_sha1_rounds_ch(out, a, u32(g.s), b);
    else if (same(v, "sha1_rounds_parity")) vertex_sha1_rounds_parity(out, a, u32(g.s), b);
    else if (same(v, "sha1_rounds_maj")) vertex_sha1_rounds_maj(out, a, u32(g.s), b);
    else if (same(v, "sha1_msg1")) vertex_sha1_msg1(out, a, b, c);
    else if (same(v, "sha1_msg2")) vertex_sha1_msg2(out, a, b);
    else if (same(v, "sha256_rounds_abcd")) vertex_sha256_rounds_abcd(out, a, b, c);
    else if (same(v, "sha256_rounds_efgh")) vertex_sha256_rounds_efgh(out, a, b, c);
    else if (same(v, "sha256_msg1")) vertex_sha256_msg1(out, a, b);
    else if (same(v, "sha256_msg2")) vertex_sha256_msg2(out, a, b, c);
    else if (same(v, "sha512_rounds_1")) vertex_sha512_rounds_1(out, a, b, c);
    else if (same(v, "sha512_rounds_2")) vertex_sha512_rounds_2(out, a, b, c);
    else if (same(v, "sha512_msg1")) vertex_sha512_msg1(out, a, b);
    else if (same(v, "sha512_msg2")) vertex_sha512_msg2(out, a, b, c);
    else {
      say(v);
      continue;
    }
    if (!equal16(out, g.want))
      say(v);
  }
  for (const GoldenCRC& g : goldenCRC) {
    const char* v = g.verb;
    u32 got = 0;
    u32 d32 = u32(g.data);
    if (same(v, "crc32c_u8")) got = vertex_crc32c_u8(g.crc, d32);
    else if (same(v, "crc32c_u16")) got = vertex_crc32c_u16(g.crc, d32);
    else if (same(v, "crc32c_u32")) got = vertex_crc32c_u32(g.crc, d32);
    else if (same(v, "crc32c_u64")) got = vertex_crc32c_u64(g.crc, g.data);
    else if (same(v, "crc32_u8")) got = vertex_crc32_u8(g.crc, d32);
    else if (same(v, "crc32_u16")) got = vertex_crc32_u16(g.crc, d32);
    else if (same(v, "crc32_u32")) got = vertex_crc32_u32(g.crc, d32);
    else if (same(v, "crc32_u64")) got = vertex_crc32_u64(g.crc, g.data);
    if (got != g.want)
      say(v);
  }

  // AES-128 through aes_keygen_assist and the rounds: FIPS-197 A.1 and B.
  {
    const u8 key[16] = {0x2b, 0x7e, 0x15, 0x16, 0x28, 0xae, 0xd2, 0xa6, 0xab, 0xf7, 0x15, 0x88, 0x09, 0xcf, 0x4f, 0x3c};
    u8 rk[11][16];
    for (int i = 0; i < 16; i++)
      rk[0][i] = key[i];
    const int rcon[10] = {1, 2, 4, 8, 16, 32, 64, 128, 0x1b, 0x36};
    for (int r = 0; r < 10; r++) {
      u8 t[16];
      vertex_aes_keygen_assist(t, rk[r], rcon[r]);
      u32 w[4];
      for (int i = 0; i < 4; i++)
        w[i] = crypto::lane32(rk[r], i);
      w[0] ^= crypto::lane32(t, 3);
      w[1] ^= w[0];
      w[2] ^= w[1];
      w[3] ^= w[2];
      for (int i = 0; i < 4; i++)
        crypto::put32(rk[r + 1], i, w[i]);
    }
    const u8 last[16] = {0xd0, 0x14, 0xf9, 0xa8, 0xc9, 0xee, 0x25, 0x89, 0xe1, 0x3f, 0x0c, 0xc8, 0xb6, 0x63, 0x0c, 0xa6};
    if (!equal16(rk[10], last))
      say("aes_keygen_assist: FIPS-197 A.1");
    const u8 pt[16] = {0x32, 0x43, 0xf6, 0xa8, 0x88, 0x5a, 0x30, 0x8d, 0x31, 0x31, 0x98, 0xa2, 0xe0, 0x37, 0x07, 0x34};
    const u8 ct[16] = {0x39, 0x25, 0x84, 0x1d, 0x02, 0xdc, 0x09, 0xfb, 0xdc, 0x11, 0x85, 0x97, 0x19, 0x6a, 0x0b, 0x32};
    u8 s[16];
    for (int i = 0; i < 16; i++)
      s[i] = u8(pt[i] ^ rk[0][i]);
    for (int r = 1; r < 10; r++)
      vertex_aes_enc(s, s, rk[r]);
    vertex_aes_enc_last(s, s, rk[10]);
    if (!equal16(s, ct))
      say("aes_enc: FIPS-197 B");
  }

  // SM4: GB/T 32907 example 1, key and plaintext 0123456789abcdeffedcba9876543210.
  {
    const u32 mk[4] = {0x01234567, 0x89abcdef, 0xfedcba98, 0x76543210};
    const u32 fk[4] = {0xa3b1bac6, 0x56aa3350, 0x677d9197, 0xb27022dc};
    u8 k[16], keys[8][16], st[16], want[16];
    for (int i = 0; i < 4; i++) {
      crypto::put32(k, i, mk[i] ^ fk[i]);
      crypto::put32(st, i, mk[i]);
    }
    for (int g = 0; g < 8; g++) {
      u8 ck[16];
      for (int i = 0; i < 4; i++) {
        u32 v = 0;
        for (int j = 0; j < 4; j++)
          v = v << 8 | u8((4 * (4 * g + i) + j) * 7);
        crypto::put32(ck, i, v);
      }
      vertex_sm4_key(keys[g], g == 0 ? k : keys[g - 1], ck);
    }
    for (int g = 0; g < 8; g++)
      vertex_sm4_rounds(st, st, keys[g]);
    const u32 ct[4] = {0x681edf34, 0xd206965e, 0x86b3e94f, 0x536e4246};
    for (int i = 0; i < 4; i++)
      crypto::put32(want, i, ct[3 - i]);
    if (!equal16(st, want))
      say("sm4: GB/T 32907");
  }
  return 0;
}
