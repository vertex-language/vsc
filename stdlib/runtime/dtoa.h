// Floating-point description: the shortest digits that read back as the
// same value, laid out the way Swift's `description` lays them out.
//
// The digits are Burger and Dybvig's free-format algorithm over exact
// big-integer arithmetic. It is not the fastest way to get them, and it
// is the one whose correctness follows from its definition: every digit
// is decided by comparing exact values, so the output is the shortest
// that round-trips and, of those, the closest.
#pragma once

#include "vertex/abi.h"
#include "text.h"

namespace vertex {

// A non-negative integer of up to 64 * 32 bits, least significant limb
// first. 2^1100 * 10^20 is the most a double needs.
struct Big {
  u32 limbs[64];
  u32 used;
};

inline void bigSet(Big& b, u64 v) {
  b.used = 0;
  while (v != 0) {
    b.limbs[b.used++] = static_cast<u32>(v);
    v >>= 32;
  }
}

inline void bigMulSmall(Big& b, u32 m) {
  u64 carry = 0;
  for (u32 i = 0; i < b.used; i++) {
    u64 p = static_cast<u64>(b.limbs[i]) * m + carry;
    b.limbs[i] = static_cast<u32>(p);
    carry = p >> 32;
  }
  if (carry != 0)
    b.limbs[b.used++] = static_cast<u32>(carry);
}

inline void bigShiftLeft(Big& b, u32 bits) {
  if (b.used == 0)
    return;
  u32 words = bits / 32, rest = bits % 32;
  if (rest != 0) {
    u32 carry = 0;
    for (u32 i = 0; i < b.used; i++) {
      u32 v = b.limbs[i];
      b.limbs[i] = (v << rest) | carry;
      carry = v >> (32 - rest);
    }
    if (carry != 0)
      b.limbs[b.used++] = carry;
  }
  if (words != 0) {
    for (u32 i = b.used; i > 0; i--)
      b.limbs[i - 1 + words] = b.limbs[i - 1];
    for (u32 i = 0; i < words; i++)
      b.limbs[i] = 0;
    b.used += words;
  }
}

inline i32 bigCompare(const Big& a, const Big& b) {
  if (a.used != b.used)
    return a.used < b.used ? -1 : 1;
  for (u32 i = a.used; i > 0; i--)
    if (a.limbs[i - 1] != b.limbs[i - 1])
      return a.limbs[i - 1] < b.limbs[i - 1] ? -1 : 1;
  return 0;
}

// a + b compared with c, without forming the sum where it would not fit.
inline i32 bigCompareSum(const Big& a, const Big& b, const Big& c) {
  Big sum;
  u32 n = a.used > b.used ? a.used : b.used;
  u64 carry = 0;
  for (u32 i = 0; i < n; i++) {
    u64 s = carry;
    if (i < a.used) s += a.limbs[i];
    if (i < b.used) s += b.limbs[i];
    sum.limbs[i] = static_cast<u32>(s);
    carry = s >> 32;
  }
  sum.used = n;
  if (carry != 0)
    sum.limbs[sum.used++] = static_cast<u32>(carry);
  return bigCompare(sum, c);
}

// a -= b, where a >= b.
inline void bigSub(Big& a, const Big& b) {
  i64 borrow = 0;
  for (u32 i = 0; i < a.used; i++) {
    i64 d = static_cast<i64>(a.limbs[i]) - borrow - (i < b.used ? static_cast<i64>(b.limbs[i]) : 0);
    borrow = d < 0 ? 1 : 0;
    a.limbs[i] = static_cast<u32>(d + (borrow << 32));
  }
  while (a.used > 0 && a.limbs[a.used - 1] == 0)
    a.used--;
}

inline void bigPow10(Big& b, u32 n) {
  for (u32 i = 0; i < n; i++)
    bigMulSmall(b, 10);
}

// shortestDigits writes the digits of f * 2^e, a finite positive value
// whose mantissa has `precision` bits, and answers how many; *exponent
// is k where the value is 0.d1d2... * 10^k.
inline u32 shortestDigits(u64 f, i32 e, i32 precision, i32 minExponent, u8* digits, i32* exponent) {
  Big r, s, mplus, mminus;
  bool evenMantissa = (f & 1) == 0;
  bool boundaryShift = f == (1ull << (precision - 1)) && e > minExponent;
  if (e >= 0) {
    bigSet(r, f);
    bigShiftLeft(r, static_cast<u32>(e) + (boundaryShift ? 2 : 1));
    bigSet(s, boundaryShift ? 4 : 2);
    bigSet(mplus, 1);
    bigShiftLeft(mplus, static_cast<u32>(e) + (boundaryShift ? 1 : 0));
    bigSet(mminus, 1);
    bigShiftLeft(mminus, static_cast<u32>(e));
  } else {
    bigSet(r, f);
    bigShiftLeft(r, boundaryShift ? 2 : 1);
    bigSet(s, 1);
    bigShiftLeft(s, static_cast<u32>(-e) + (boundaryShift ? 2 : 1));
    bigSet(mplus, boundaryShift ? 2 : 1);
    bigSet(mminus, 1);
  }

  // An estimate of k no larger than the answer: the value is at least
  // 2^(bits-1), and log10(2) is between 78913/2^18 and 78914/2^18.
  // Counted rather than asked of clz, which the x86-64 baseline does not
  // have as an instruction.
  i32 bits = e;
  for (u64 m = f; m != 0; m >>= 1)
    bits++;
  i32 k = (bits - 1 >= 0 ? ((bits - 1) * 78913) >> 18 : ((bits - 1) * 78914) >> 18) + 1;
  if (k >= 0)
    bigPow10(s, static_cast<u32>(k));
  else {
    bigPow10(r, static_cast<u32>(-k));
    bigPow10(mplus, static_cast<u32>(-k));
    bigPow10(mminus, static_cast<u32>(-k));
  }
  for (;;) {
    i32 c = bigCompareSum(r, mplus, s);
    if (evenMantissa ? c < 0 : c <= 0)
      break;
    bigMulSmall(s, 10);
    k++;
  }
  *exponent = k;

  u32 n = 0;
  for (;;) {
    bigMulSmall(r, 10);
    bigMulSmall(mplus, 10);
    bigMulSmall(mminus, 10);
    u8 d = 0;
    while (bigCompare(r, s) >= 0) {
      bigSub(r, s);
      d++;
    }
    i32 low = bigCompare(r, mminus);
    i32 high = bigCompareSum(r, mplus, s);
    bool tc1 = evenMantissa ? low <= 0 : low < 0;
    bool tc2 = evenMantissa ? high >= 0 : high > 0;
    if (!tc1 && !tc2) {
      digits[n++] = d;
      continue;
    }
    if (tc1 && tc2) {
      Big twice = r;
      bigMulSmall(twice, 2);
      i32 c = bigCompare(twice, s);
      if (c > 0 || (c == 0 && (d & 1) != 0))
        d++;
    } else if (tc2) {
      d++;
    }
    digits[n++] = d;
    break;
  }
  return n;
}

// describeFloat writes a binary floating-point value's description:
// sign, then fixed notation, or exponent notation for a value past
// 2^precision in magnitude or smaller than 10^-4.
inline void describeFloat(Text& t, u64 bits, i32 precision, i32 exponentBits) {
  u64 mantissaMask = (1ull << (precision - 1)) - 1;
  i32 bias = (1 << (exponentBits - 1)) - 1;
  bool negative = (bits >> (precision - 1 + exponentBits)) & 1;
  u64 mantissa = bits & mantissaMask;
  i32 biased = static_cast<i32>((bits >> (precision - 1)) & ((1u << exponentBits) - 1));

  if (biased == (1 << exponentBits) - 1) {
    if (mantissa != 0) {
      textString(t, "nan");
      return;
    }
    textString(t, negative ? "-inf" : "inf");
    return;
  }
  if (negative)
    textByte(t, '-');
  if (biased == 0 && mantissa == 0) {
    textString(t, "0.0");
    return;
  }

  i32 minExponent = 1 - bias - (precision - 1);
  u64 f;
  i32 e;
  if (biased == 0) {
    f = mantissa;
    e = minExponent;
  } else {
    f = mantissa | (1ull << (precision - 1));
    e = biased - bias - (precision - 1);
  }

  // A half that is a whole number -- past 2^11 every one is -- is
  // written exactly, as Swift writes 65504.0 and not 65500.0.
  // bfloat16 has Float's exponent, and follows Float's rules instead.
  if (precision < 24 && exponentBits < 8 && e >= 0) {
    textUnsigned(t, f << e);
    textString(t, ".0");
    return;
  }

  u8 digits[32];
  i32 k = 0;
  u32 n = shortestDigits(f, e, precision, minExponent, digits, &k);

  // Past 2^precision, where not every integer is representable -- and
  // never for a half, whose every finite value Swift writes in decimal.
  i32 limit = precision < 24 ? 24 : precision;
  bool huge = biased != 0 && (biased - bias > limit ||
                              (biased - bias == limit && mantissa != 0));
  if (huge || k < -3) {
    textByte(t, static_cast<u8>('0' + digits[0]));
    if (n > 1) {
      textByte(t, '.');
      for (u32 i = 1; i < n; i++)
        textByte(t, static_cast<u8>('0' + digits[i]));
    }
    i32 exp10 = k - 1;
    textByte(t, 'e');
    textByte(t, exp10 < 0 ? '-' : '+');
    u32 mag = static_cast<u32>(exp10 < 0 ? -exp10 : exp10);
    if (mag < 10)
      textByte(t, '0');
    textUnsigned(t, mag);
    return;
  }
  if (k <= 0) {
    textString(t, "0.");
    for (i32 i = 0; i < -k; i++)
      textByte(t, '0');
    for (u32 i = 0; i < n; i++)
      textByte(t, static_cast<u8>('0' + digits[i]));
    return;
  }
  for (u32 i = 0; i < n; i++) {
    if (static_cast<i32>(i) == k)
      textByte(t, '.');
    textByte(t, static_cast<u8>('0' + digits[i]));
  }
  if (static_cast<i32>(n) <= k) {
    for (i32 i = static_cast<i32>(n); i < k; i++)
      textByte(t, '0');
    textString(t, ".0");
  }
}

} // namespace vertex
