// §V5's and §A8's instructions, for a processor without them.
//
// ir's LegalizeCrypto makes a call of vertex_<verb> wherever a function may
// not execute one of these verbs: the v128 operands and the result travel
// through memory, sixteen bytes each in memory order, and the scalars as
// themselves (see ir/legalize_crypto.go). Each routine here answers what
// the instruction would -- the definitions are the IR spec's, which are
// Intel's for the AES rounds and Arm's, pseudocode for pseudocode, for the
// rest -- and none of them branches on or indexes by its operands: an
// S-box is computed rather than looked up, and a bit decides by a mask.
// They are slow; the point of them is that a program runs, and keeps its
// secrets, on a processor without the instruction.
//
// Lanes are little-endian: lane i of an i32x4 is bytes 4i to 4i+3.

namespace vertex {
namespace crypto {

static inline u32 lane32(const u8* v, int i) {
  const u8* p = v + 4 * i;
  return u32(p[0]) | u32(p[1]) << 8 | u32(p[2]) << 16 | u32(p[3]) << 24;
}

static inline void put32(u8* v, int i, u32 x) {
  u8* p = v + 4 * i;
  p[0] = u8(x);
  p[1] = u8(x >> 8);
  p[2] = u8(x >> 16);
  p[3] = u8(x >> 24);
}

static inline u64 lane64(const u8* v, int i) {
  return u64(lane32(v, 2 * i)) | u64(lane32(v, 2 * i + 1)) << 32;
}

static inline void put64(u8* v, int i, u64 x) {
  put32(v, 2 * i, u32(x));
  put32(v, 2 * i + 1, u32(x >> 32));
}

static inline u32 rol32(u32 x, int n) { return n == 0 ? x : (x << n) | (x >> (32 - n)); }
static inline u32 ror32(u32 x, int n) { return rol32(x, (32 - n) & 31); }
static inline u64 ror64(u64 x, int n) { return n == 0 ? x : (x >> n) | (x << (64 - n)); }
static inline u8 rol8(u8 x, int n) { return u8((x << n) | (x >> (8 - n))); }

// mask is all ones where bit is one, from the bit and not a branch.
static inline u32 mask32(u32 bit) { return 0u - (bit & 1); }
static inline u64 mask64(u64 bit) { return 0ull - (bit & 1); }

// ---- GF(2^8), for the AES and SM4 S-boxes ----

// gmul is a times b modulo the field's polynomial (its low eight bits,
// the x^8 implied), in eight steps whatever the operands.
static inline u8 gmul(u8 a, u8 b, u8 poly) {
  u8 p = 0;
  for (int i = 0; i < 8; i++) {
    p ^= u8(mask32(b >> i) & a);
    a = u8((a << 1) ^ (mask32(a >> 7) & poly));
  }
  return p;
}

// ginv is a's inverse, a^254, and 0 for 0: the exponent is fixed, so the
// sequence of multiplications is too.
static inline u8 ginv(u8 a, u8 poly) {
  u8 r = 1, x = a;
  for (int i = 0; i < 8; i++) {
    if (i != 0) // 254 is 0b11111110: every bit but the lowest
      r = gmul(r, x, poly);
    x = gmul(x, x, poly);
  }
  return r;
}

// The AES field is x^8 + x^4 + x^3 + x + 1.
static inline u8 aesSbox(u8 x) {
  u8 b = ginv(x, 0x1b);
  return u8(b ^ rol8(b, 1) ^ rol8(b, 2) ^ rol8(b, 3) ^ rol8(b, 4) ^ 0x63);
}

static inline u8 aesInvSbox(u8 y) { return ginv(u8(rol8(y, 1) ^ rol8(y, 3) ^ rol8(y, 6) ^ 0x05), 0x1b); }

// SM4's is A·inv(A·x + C) + C, over x^8 + x^7 + x^6 + x^5 + x^4 + x^2 + 1,
// with A the circulant matrix of 0xA7 and C = 0xD3: bit i of A·x is the
// parity of x and 0xA7 rotated left by i.
static inline u8 sm4Affine(u8 x) {
  u8 out = 0;
  for (int i = 0; i < 8; i++) {
    u8 v = u8(rol8(0xA7, i) & x);
    v ^= v >> 4;
    v ^= v >> 2;
    v ^= v >> 1;
    out |= u8((v & 1) << i);
  }
  return u8(out ^ 0xD3);
}

static inline u8 sm4Sbox(u8 x) { return sm4Affine(ginv(sm4Affine(x), 0xF5)); }

// ---- AES ----

static inline u8 xtime(u8 a) { return u8((a << 1) ^ (mask32(a >> 7) & 0x1b)); }

static void subBytes(u8* s) {
  for (int i = 0; i < 16; i++)
    s[i] = aesSbox(s[i]);
}

static void invSubBytes(u8* s) {
  for (int i = 0; i < 16; i++)
    s[i] = aesInvSbox(s[i]);
}

// Byte 4c + r is row r of column c.
static void shiftRows(u8* s) {
  u8 t[16];
  for (int c = 0; c < 4; c++)
    for (int r = 0; r < 4; r++)
      t[4 * c + r] = s[4 * ((c + r) & 3) + r];
  for (int i = 0; i < 16; i++)
    s[i] = t[i];
}

static void invShiftRows(u8* s) {
  u8 t[16];
  for (int c = 0; c < 4; c++)
    for (int r = 0; r < 4; r++)
      t[4 * c + r] = s[4 * ((c - r + 4) & 3) + r];
  for (int i = 0; i < 16; i++)
    s[i] = t[i];
}

static void mixColumns(u8* s) {
  for (int c = 0; c < 4; c++) {
    u8* p = s + 4 * c;
    u8 a0 = p[0], a1 = p[1], a2 = p[2], a3 = p[3];
    u8 all = u8(a0 ^ a1 ^ a2 ^ a3);
    p[0] = u8(a0 ^ all ^ xtime(u8(a0 ^ a1)));
    p[1] = u8(a1 ^ all ^ xtime(u8(a1 ^ a2)));
    p[2] = u8(a2 ^ all ^ xtime(u8(a2 ^ a3)));
    p[3] = u8(a3 ^ all ^ xtime(u8(a3 ^ a0)));
  }
}

static void invMixColumns(u8* s) {
  for (int c = 0; c < 4; c++) {
    u8* p = s + 4 * c;
    u8 a0 = p[0], a1 = p[1], a2 = p[2], a3 = p[3];
    p[0] = u8(gmul(a0, 14, 0x1b) ^ gmul(a1, 11, 0x1b) ^ gmul(a2, 13, 0x1b) ^ gmul(a3, 9, 0x1b));
    p[1] = u8(gmul(a0, 9, 0x1b) ^ gmul(a1, 14, 0x1b) ^ gmul(a2, 11, 0x1b) ^ gmul(a3, 13, 0x1b));
    p[2] = u8(gmul(a0, 13, 0x1b) ^ gmul(a1, 9, 0x1b) ^ gmul(a2, 14, 0x1b) ^ gmul(a3, 11, 0x1b));
    p[3] = u8(gmul(a0, 11, 0x1b) ^ gmul(a1, 13, 0x1b) ^ gmul(a2, 9, 0x1b) ^ gmul(a3, 14, 0x1b));
  }
}

static void copy16(u8* d, const u8* s) {
  for (int i = 0; i < 16; i++)
    d[i] = s[i];
}

static void xor16(u8* d, const u8* k) {
  for (int i = 0; i < 16; i++)
    d[i] ^= k[i];
}

static inline u32 subWord(u32 w) {
  return u32(aesSbox(u8(w))) | u32(aesSbox(u8(w >> 8))) << 8 | u32(aesSbox(u8(w >> 16))) << 16 |
         u32(aesSbox(u8(w >> 24))) << 24;
}

// ---- SHA ----

static inline u32 choose(u32 x, u32 y, u32 z) { return ((y ^ z) & x) ^ z; }
static inline u32 parity(u32 x, u32 y, u32 z) { return x ^ y ^ z; }
static inline u32 majority(u32 x, u32 y, u32 z) { return (x & y) | ((x | y) & z); }

// SHA1C, SHA1P and SHA1M: four rounds of choose (0), parity (1) or
// majority (2) on X = abcd and Y = e. Which is the instruction's, not a
// secret, so it is chosen by a branch.
static void sha1Rounds(u8* out, const u8* abcd, u32 e, const u8* wk, int f) {
  u32 x[4] = {lane32(abcd, 0), lane32(abcd, 1), lane32(abcd, 2), lane32(abcd, 3)};
  u32 y = e;
  for (int i = 0; i < 4; i++) {
    u32 t = f == 0 ? choose(x[1], x[2], x[3]) : f == 1 ? parity(x[1], x[2], x[3]) : majority(x[1], x[2], x[3]);
    y = y + rol32(x[0], 5) + t + lane32(wk, i);
    x[1] = rol32(x[1], 30);
    // <Y, X> = ROL(Y:X, 32)
    u32 top = x[3];
    x[3] = x[2];
    x[2] = x[1];
    x[1] = x[0];
    x[0] = y;
    y = top;
  }
  for (int i = 0; i < 4; i++)
    put32(out, i, x[i]);
}

static inline u32 bigSigma0(u32 x) { return ror32(x, 2) ^ ror32(x, 13) ^ ror32(x, 22); }
static inline u32 bigSigma1(u32 x) { return ror32(x, 6) ^ ror32(x, 11) ^ ror32(x, 25); }
static inline u32 smallSigma0(u32 x) { return ror32(x, 7) ^ ror32(x, 18) ^ (x >> 3); }
static inline u32 smallSigma1(u32 x) { return ror32(x, 17) ^ ror32(x, 19) ^ (x >> 10); }

// SHA256hash: four rounds on X = abcd and Y = efgh; part1 answers X.
static void sha256Hash(u8* out, const u8* abcd, const u8* efgh, const u8* wk, bool part1) {
  u32 x[4], y[4];
  for (int i = 0; i < 4; i++) {
    x[i] = lane32(abcd, i);
    y[i] = lane32(efgh, i);
  }
  for (int i = 0; i < 4; i++) {
    u32 chs = choose(y[0], y[1], y[2]);
    u32 maj = majority(x[0], x[1], x[2]);
    u32 t = y[3] + bigSigma1(y[0]) + chs + lane32(wk, i);
    x[3] = t + x[3];
    y[3] = t + bigSigma0(x[0]) + maj;
    // <Y, X> = ROL(Y:X, 32)
    u32 xTop = x[3], yTop = y[3];
    x[3] = x[2], x[2] = x[1], x[1] = x[0], x[0] = yTop;
    y[3] = y[2], y[2] = y[1], y[1] = y[0], y[0] = xTop;
  }
  for (int i = 0; i < 4; i++)
    put32(out, i, part1 ? x[i] : y[i]);
}

// ---- SM3 and SM4 ----

static inline u32 sm4Tau(u32 x) {
  return u32(sm4Sbox(u8(x))) | u32(sm4Sbox(u8(x >> 8))) << 8 | u32(sm4Sbox(u8(x >> 16))) << 16 |
         u32(sm4Sbox(u8(x >> 24))) << 24;
}

} // namespace crypto
} // namespace vertex

extern "C" {

using namespace vertex;
using namespace vertex::crypto;

// ---- AES ----

void vertex_aes_enc(u8* out, const u8* s, const u8* k) {
  u8 t[16];
  copy16(t, s);
  subBytes(t);
  shiftRows(t);
  mixColumns(t);
  xor16(t, k);
  copy16(out, t);
}

void vertex_aes_enc_last(u8* out, const u8* s, const u8* k) {
  u8 t[16];
  copy16(t, s);
  subBytes(t);
  shiftRows(t);
  xor16(t, k);
  copy16(out, t);
}

void vertex_aes_dec(u8* out, const u8* s, const u8* k) {
  u8 t[16];
  copy16(t, s);
  invShiftRows(t);
  invSubBytes(t);
  invMixColumns(t);
  xor16(t, k);
  copy16(out, t);
}

void vertex_aes_dec_last(u8* out, const u8* s, const u8* k) {
  u8 t[16];
  copy16(t, s);
  invShiftRows(t);
  invSubBytes(t);
  xor16(t, k);
  copy16(out, t);
}

void vertex_aes_inv_mix_columns(u8* out, const u8* s) {
  u8 t[16];
  copy16(t, s);
  invMixColumns(t);
  copy16(out, t);
}

// RotWord is a rotation right by eight of the little-endian word.
void vertex_aes_keygen_assist(u8* out, const u8* x, i32 rcon) {
  u32 x1 = subWord(lane32(x, 1)), x3 = subWord(lane32(x, 3));
  put32(out, 0, x1);
  put32(out, 1, ror32(x1, 8) ^ u32(rcon & 0xff));
  put32(out, 2, x3);
  put32(out, 3, ror32(x3, 8) ^ u32(rcon & 0xff));
}

// ---- Carry-less multiplication ----

void vertex_i64x2_clmul(u8* out, const u8* a, const u8* b, i32 sel) {
  u64 x = lane64(a, sel & 1), y = lane64(b, (sel >> 1) & 1);
  u64 lo = 0, hi = 0;
  for (int i = 0; i < 64; i++) {
    u64 m = mask64(y >> i);
    lo ^= (x << i) & m;
    hi ^= (i == 0 ? 0 : x >> (64 - i)) & m;
  }
  put64(out, 0, lo);
  put64(out, 1, hi);
}

// ---- SHA-1 ----

void vertex_sha1_rounds_ch(u8* out, const u8* abcd, u32 e, const u8* wk) {
  sha1Rounds(out, abcd, e, wk, 0);
}
void vertex_sha1_rounds_parity(u8* out, const u8* abcd, u32 e, const u8* wk) {
  sha1Rounds(out, abcd, e, wk, 1);
}
void vertex_sha1_rounds_maj(u8* out, const u8* abcd, u32 e, const u8* wk) {
  sha1Rounds(out, abcd, e, wk, 2);
}

// SHA1SU0: T = Vn<63:0>:Vd<127:64>, and T ^ Vd ^ Vm.
void vertex_sha1_msg1(u8* out, const u8* w0, const u8* w4, const u8* w8) {
  u32 t[4] = {lane32(w0, 2), lane32(w0, 3), lane32(w4, 0), lane32(w4, 1)};
  for (int i = 0; i < 4; i++)
    put32(out, i, t[i] ^ lane32(w0, i) ^ lane32(w8, i));
}

// SHA1SU1: T = Vd ^ (Vn >> 32), each lane rotated by one, and lane 3 also
// takes lane 0's -- W[t+16], which it depends on -- rotated by two.
void vertex_sha1_msg2(u8* out, const u8* x, const u8* w12) {
  u32 t[4] = {lane32(x, 0) ^ lane32(w12, 1), lane32(x, 1) ^ lane32(w12, 2), lane32(x, 2) ^ lane32(w12, 3),
              lane32(x, 3)};
  put32(out, 0, rol32(t[0], 1));
  put32(out, 1, rol32(t[1], 1));
  put32(out, 2, rol32(t[2], 1));
  put32(out, 3, rol32(t[3], 1) ^ rol32(t[0], 2));
}

// ---- SHA-256 ----

void vertex_sha256_rounds_abcd(u8* out, const u8* abcd, const u8* efgh, const u8* wk) {
  sha256Hash(out, abcd, efgh, wk, true);
}

void vertex_sha256_rounds_efgh(u8* out, const u8* efgh, const u8* abcd, const u8* wk) {
  sha256Hash(out, abcd, efgh, wk, false);
}

// SHA256SU0: T = Vn<31:0>:Vd<127:32>, and Vd + σ0(T).
void vertex_sha256_msg1(u8* out, const u8* w0, const u8* w4) {
  u32 t[4] = {lane32(w0, 1), lane32(w0, 2), lane32(w0, 3), lane32(w4, 0)};
  for (int i = 0; i < 4; i++)
    put32(out, i, lane32(w0, i) + smallSigma0(t[i]));
}

// SHA256SU1: T0 = W[t+9..t+12]; lanes 0 and 1 take σ1 of W[t+14] and
// W[t+15], lanes 2 and 3 of the two just computed.
void vertex_sha256_msg2(u8* out, const u8* x, const u8* w8, const u8* w12) {
  u32 t0[4] = {lane32(w8, 1), lane32(w8, 2), lane32(w8, 3), lane32(w12, 0)};
  u32 r[4];
  r[0] = smallSigma1(lane32(w12, 2)) + lane32(x, 0) + t0[0];
  r[1] = smallSigma1(lane32(w12, 3)) + lane32(x, 1) + t0[1];
  r[2] = smallSigma1(r[0]) + lane32(x, 2) + t0[2];
  r[3] = smallSigma1(r[1]) + lane32(x, 3) + t0[3];
  for (int i = 0; i < 4; i++)
    put32(out, i, r[i]);
}

// ---- SHA-512: Arm's SHA512H, SHA512H2, SHA512SU0, SHA512SU1, with the
// destination's prior value first. ----

void vertex_sha512_rounds_1(u8* out, const u8* d, const u8* n, const u8* m) {
  u64 x0 = lane64(n, 0), x1 = lane64(n, 1), y0 = lane64(m, 0), y1 = lane64(m, 1);
  u64 w0 = lane64(d, 0), w1 = lane64(d, 1);
  u64 sigma = ror64(y1, 14) ^ ror64(y1, 18) ^ ror64(y1, 41);
  u64 hi = ((y1 & x0) ^ (~y1 & x1)) + sigma + w1;
  u64 tmp = hi + y0;
  sigma = ror64(tmp, 14) ^ ror64(tmp, 18) ^ ror64(tmp, 41);
  u64 lo = ((tmp & y1) ^ (~tmp & x0)) + sigma + w0;
  put64(out, 0, lo);
  put64(out, 1, hi);
}

void vertex_sha512_rounds_2(u8* out, const u8* d, const u8* n, const u8* m) {
  u64 x0 = lane64(n, 0), y0 = lane64(m, 0), y1 = lane64(m, 1);
  u64 w0 = lane64(d, 0), w1 = lane64(d, 1);
  u64 sigma = ror64(y0, 28) ^ ror64(y0, 34) ^ ror64(y0, 39);
  u64 hi = ((x0 & y1) ^ (x0 & y0) ^ (y1 & y0)) + sigma + w1;
  sigma = ror64(hi, 28) ^ ror64(hi, 34) ^ ror64(hi, 39);
  u64 lo = ((hi & y0) ^ (hi & y1) ^ (y1 & y0)) + sigma + w0;
  put64(out, 0, lo);
  put64(out, 1, hi);
}

void vertex_sha512_msg1(u8* out, const u8* d, const u8* n) {
  u64 w0 = lane64(d, 0), w1 = lane64(d, 1), x0 = lane64(n, 0);
  put64(out, 0, w0 + (ror64(w1, 1) ^ ror64(w1, 8) ^ (w1 >> 7)));
  put64(out, 1, w1 + (ror64(x0, 1) ^ ror64(x0, 8) ^ (x0 >> 7)));
}

void vertex_sha512_msg2(u8* out, const u8* d, const u8* n, const u8* m) {
  u64 x0 = lane64(n, 0), x1 = lane64(n, 1);
  put64(out, 1, lane64(d, 1) + (ror64(x1, 19) ^ ror64(x1, 61) ^ (x1 >> 6)) + lane64(m, 1));
  put64(out, 0, lane64(d, 0) + (ror64(x0, 19) ^ ror64(x0, 61) ^ (x0 >> 6)) + lane64(m, 0));
}

// ---- SM3: Arm's, the destination's prior value first where it is read ----

void vertex_sm3_ss1(u8* out, const u8* n, const u8* m, const u8* a) {
  u32 r = rol32(rol32(lane32(n, 3), 12) + lane32(m, 3) + lane32(a, 3), 7);
  put32(out, 0, 0);
  put32(out, 1, 0);
  put32(out, 2, 0);
  put32(out, 3, r);
}

static void sm3tt1(u8* out, const u8* d, const u8* n, const u8* m, i32 lane, bool majorityForm) {
  u32 wj = lane32(m, lane & 3);
  u32 d0 = lane32(d, 0), d1 = lane32(d, 1), d2 = lane32(d, 2), d3 = lane32(d, 3);
  u32 tt1 = majorityForm ? (d3 & d2) | (d3 & d1) | (d2 & d1) : d3 ^ d2 ^ d1;
  tt1 = tt1 + d0 + (lane32(n, 3) ^ rol32(d3, 12)) + wj;
  put32(out, 0, d1);
  put32(out, 1, rol32(d2, 9));
  put32(out, 2, d3);
  put32(out, 3, tt1);
}

static void sm3tt2(u8* out, const u8* d, const u8* n, const u8* m, i32 lane, bool chooseForm) {
  u32 wj = lane32(m, lane & 3);
  u32 d0 = lane32(d, 0), d1 = lane32(d, 1), d2 = lane32(d, 2), d3 = lane32(d, 3);
  u32 tt2 = chooseForm ? (d3 & d2) | (~d3 & d1) : d3 ^ d2 ^ d1;
  tt2 = tt2 + d0 + lane32(n, 3) + wj;
  put32(out, 0, d1);
  put32(out, 1, rol32(d2, 19));
  put32(out, 2, d3);
  put32(out, 3, tt2 ^ rol32(tt2, 9) ^ rol32(tt2, 17));
}

void vertex_sm3_tt1a(u8* out, const u8* d, const u8* n, const u8* m, i32 lane) { sm3tt1(out, d, n, m, lane, false); }
void vertex_sm3_tt1b(u8* out, const u8* d, const u8* n, const u8* m, i32 lane) { sm3tt1(out, d, n, m, lane, true); }
void vertex_sm3_tt2a(u8* out, const u8* d, const u8* n, const u8* m, i32 lane) { sm3tt2(out, d, n, m, lane, false); }
void vertex_sm3_tt2b(u8* out, const u8* d, const u8* n, const u8* m, i32 lane) { sm3tt2(out, d, n, m, lane, true); }

void vertex_sm3_partw1(u8* out, const u8* d, const u8* n, const u8* m) {
  u32 r[4];
  r[0] = (lane32(d, 0) ^ lane32(n, 0)) ^ rol32(lane32(m, 1), 15);
  r[1] = (lane32(d, 1) ^ lane32(n, 1)) ^ rol32(lane32(m, 2), 15);
  r[2] = (lane32(d, 2) ^ lane32(n, 2)) ^ rol32(lane32(m, 3), 15);
  for (int i = 0; i < 4; i++) {
    if (i == 3)
      r[3] = (lane32(d, 3) ^ lane32(n, 3)) ^ rol32(r[0], 15);
    r[i] = r[i] ^ rol32(r[i], 15) ^ rol32(r[i], 23);
  }
  for (int i = 0; i < 4; i++)
    put32(out, i, r[i]);
}

void vertex_sm3_partw2(u8* out, const u8* d, const u8* n, const u8* m) {
  u32 tmp[4], r[4];
  for (int i = 0; i < 4; i++) {
    tmp[i] = lane32(n, i) ^ rol32(lane32(m, i), 7);
    r[i] = lane32(d, i) ^ tmp[i];
  }
  u32 t2 = rol32(tmp[0], 15);
  t2 = t2 ^ rol32(t2, 15) ^ rol32(t2, 23);
  r[3] ^= t2;
  for (int i = 0; i < 4; i++)
    put32(out, i, r[i]);
}

// ---- SM4 ----

void vertex_sm4_rounds(u8* out, const u8* state, const u8* rk) {
  u32 x[4] = {lane32(state, 0), lane32(state, 1), lane32(state, 2), lane32(state, 3)};
  for (int i = 0; i < 4; i++) {
    u32 v = sm4Tau(x[3] ^ x[2] ^ x[1] ^ lane32(rk, i));
    v = v ^ rol32(v, 2) ^ rol32(v, 10) ^ rol32(v, 18) ^ rol32(v, 24);
    v ^= x[0];
    x[0] = x[1], x[1] = x[2], x[2] = x[3], x[3] = v;
  }
  for (int i = 0; i < 4; i++)
    put32(out, i, x[i]);
}

void vertex_sm4_key(u8* out, const u8* rk, const u8* ck) {
  u32 x[4] = {lane32(rk, 0), lane32(rk, 1), lane32(rk, 2), lane32(rk, 3)};
  for (int i = 0; i < 4; i++) {
    u32 v = sm4Tau(x[3] ^ x[2] ^ x[1] ^ lane32(ck, i));
    v = v ^ rol32(v, 13) ^ rol32(v, 23);
    v ^= x[0];
    x[0] = x[1], x[1] = x[2], x[2] = x[3], x[3] = v;
  }
  for (int i = 0; i < 4; i++)
    put32(out, i, x[i]);
}

// ---- i8x16_swizzle, for a target with no byte permute (ExpandVectorVerbs) ----

// Each byte of the table by each index, zero for an index of 16 or more:
// every byte of the table is read for every index, so no address depends
// on one.
void vertex_i8x16_swizzle(u8* out, const u8* table, const u8* index) {
  u8 r[16];
  for (int i = 0; i < 16; i++) {
    u8 acc = 0;
    for (int j = 0; j < 16; j++)
      acc |= u8(table[j] & u8(0u - u32(index[i] == j)));
    r[i] = acc;
  }
  copy16(out, r);
}

// ---- CRC-32C and CRC-32, reflected, no inversion ----

static u32 crcBits(u32 crc, u64 data, int bits, u32 poly) {
  crc ^= u32(data);
  u64 hi = bits == 64 ? data >> 32 : 0;
  for (int i = 0; i < bits; i++) {
    if (i == 32)
      crc ^= u32(hi);
    crc = (crc >> 1) ^ (mask32(crc) & poly);
  }
  return crc;
}

u32 vertex_crc32c_u8(u32 crc, u32 data) { return crcBits(crc, data & 0xff, 8, 0x82F63B78u); }
u32 vertex_crc32c_u16(u32 crc, u32 data) { return crcBits(crc, data & 0xffff, 16, 0x82F63B78u); }
u32 vertex_crc32c_u32(u32 crc, u32 data) { return crcBits(crc, data, 32, 0x82F63B78u); }
u32 vertex_crc32c_u64(u32 crc, u64 data) { return crcBits(crc, data, 64, 0x82F63B78u); }
u32 vertex_crc32_u8(u32 crc, u32 data) { return crcBits(crc, data & 0xff, 8, 0xEDB88320u); }
u32 vertex_crc32_u16(u32 crc, u32 data) { return crcBits(crc, data & 0xffff, 16, 0xEDB88320u); }
u32 vertex_crc32_u32(u32 crc, u32 data) { return crcBits(crc, data, 32, 0xEDB88320u); }
u32 vertex_crc32_u64(u32 crc, u64 data) { return crcBits(crc, data, 64, 0xEDB88320u); }

// ---- §P ----

#if defined(__x86_64__)
// cpuid, from the runtime's assembly: eax, ebx, ecx and edx of a leaf.
void vertex_cpuid(u32 leaf, u32 subleaf, u32* out);

// Leaf 1's ecx: PCLMULQDQ (1), SSE4.2 (20, which is crc32c), AES-NI (25);
// leaf 7's ebx: SHA (29), which is SHA-1 and SHA-256. x86-64 has no CRC-32
// instruction, no 128-bit SHA-512, and no SM3 the IR uses; its SM4 is VEX,
// which this does not claim until the runtime asks the OS about AVX state.
static u32 detectCapabilities() {
  u32 r[4];
  vertex_cpuid(0, 0, r);
  u32 top = r[0];
  vertex_cpuid(1, 0, r);
  u32 ecx = r[2];
  u32 ebx7 = 0;
  if (top >= 7) {
    vertex_cpuid(7, 0, r);
    ebx7 = r[1];
  }
  u32 c = 0;
  c |= ((ecx >> 25) & 1) << 1;
  c |= ((ecx >> 1) & 1) << 2;
  c |= ((ebx7 >> 29) & 1) << 3;
  c |= ((ebx7 >> 29) & 1) << 4;
  c |= ((ecx >> 20) & 1) << 9;
  return c;
}
#else
static u32 detectCapabilities() { return vertex_pal_cpu_capabilities(); }
#endif

// The processor's answer, bit c for capability c, with bit 31 set once it
// has been asked: a race between two first askers asks twice and stores
// the same word.
static u32 vertexCapabilities = 0;

i32 vertex_cpu_has(i32 c) {
  u32 w = __builtin_atomic_load(&vertexCapabilities);
  if ((w & 0x80000000u) == 0) {
    w = detectCapabilities() | 0x80000000u;
    __builtin_atomic_store(&vertexCapabilities, w);
  }
  return i32((w >> (c & 31)) & 1);
}

} // extern "C"
