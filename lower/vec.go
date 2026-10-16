package lower

// The core's v128 builtins and checksums (core.swift's "Vectors and
// cryptography"), each one IR verb of §V, §V5, §A8 or §P. The builtin is
// named v128_<verb>_<first operand's machine>, crc_<verb>_<...> or
// cpu_has_<...>; the verb is the IR's, so this file is a table from one
// spelling to the builder that emits it, and nothing is decided here
// that the IR has not.

import (
	"strings"

	"github.com/vertex-language/ir"
)

// vecLiteral is the literal an operand must be: an instruction whose
// operand is a lane, a pattern or a round constant takes it written into
// the instruction, which a variable cannot be.
func (c *fn) vecLiteral(name string, v ir.Value) (int64, error) {
	var d *ir.Def
	switch x := v.(type) {
	case ir.I32:
		d = x.Def()
	case ir.I64:
		d = x.Def()
	}
	if d != nil && d.Inst() != nil && d.Inst().Op().Verb == ir.VConst {
		if k, ok := d.Inst().Lit(); ok && k.Kind() == ir.ConstInt {
			return k.Int(), nil
		}
	}
	return 0, c.fail(ErrBuiltin, "builtin", name+": its last operand must be written as a literal")
}

// vecBuiltin lowers a v128_, crc_ or cpu_has builtin, and reports whether
// name is one.
func (c *fn) vecBuiltin(name string, args []ir.Value) ([]ir.Value, bool, error) {
	switch {
	case strings.HasPrefix(name, "cpu_has_"):
		k, err := c.vecLiteral(name, args[0])
		if err != nil {
			return nil, true, err
		}
		capa := ir.Capability(k)
		if !capa.Valid() {
			return nil, true, c.fail(ErrBuiltin, "builtin", name+": no such capability")
		}
		return []ir.Value{c.b.I1.CpuHas(capa)}, true, nil
	case strings.HasPrefix(name, "crc_"):
		verb, _, _ := splitBuiltin(strings.TrimPrefix(name, "crc_"))
		return c.crcBuiltin(name, verb, args)
	case strings.HasPrefix(name, "v128_"):
		verb, _, _ := splitBuiltin(strings.TrimPrefix(name, "v128_"))
		return c.v128Builtin(name, verb, args)
	}
	return nil, false, nil
}

func (c *fn) crcBuiltin(name, verb string, args []ir.Value) ([]ir.Value, bool, error) {
	crc, ok := args[0].(ir.I32)
	if !ok || len(args) != 2 {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": operands")
	}
	ns := c.b.I32
	if d, ok := args[1].(ir.I64); ok {
		switch verb {
		case "crc32c_u64":
			return []ir.Value{ns.Crc32cU64(crc, d)}, true, nil
		case "crc32_u64":
			return []ir.Value{ns.Crc32U64(crc, d)}, true, nil
		}
	}
	d, ok := args[1].(ir.I32)
	if !ok {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": operands")
	}
	f := map[string]func(ir.I32, ir.I32) ir.I32{
		"crc32c_u8": ns.Crc32cU8, "crc32c_u16": ns.Crc32cU16, "crc32c_u32": ns.Crc32cU32,
		"crc32_u8": ns.Crc32U8, "crc32_u16": ns.Crc32U16, "crc32_u32": ns.Crc32U32,
	}[verb]
	if f == nil {
		return nil, true, c.fail(ErrBuiltin, "builtin", name)
	}
	return []ir.Value{f(crc, d)}, true, nil
}

func (c *fn) v128Builtin(name, verb string, args []ir.Value) ([]ir.Value, bool, error) {
	v := c.b.V128()
	vec := func(i int) (ir.V128, bool) {
		if i >= len(args) {
			return ir.V128{}, false
		}
		x, ok := args[i].(ir.V128)
		return x, ok
	}
	bad := func() ([]ir.Value, bool, error) {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": operands")
	}
	lit := func(i int) (int64, bool, error) {
		if i >= len(args) {
			return 0, false, nil
		}
		k, err := c.vecLiteral(name, args[i])
		return k, err == nil, err
	}
	one := func(x ir.Value) ([]ir.Value, bool, error) { return []ir.Value{x}, true, nil }
	a, aok := vec(0)
	b, bok := vec(1)
	d, dok := vec(2)
	switch verb {
	case "and":
		if !aok || !bok {
			return bad()
		}
		return one(v.And(a, b))
	case "or":
		if !aok || !bok {
			return bad()
		}
		return one(v.Or(a, b))
	case "xor":
		if !aok || !bok {
			return bad()
		}
		return one(v.Xor(a, b))
	case "andnot":
		if !aok || !bok {
			return bad()
		}
		return one(v.AndNot(a, b))
	case "i8x16_add":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16Add(a, b))
	case "i16x8_add":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8Add(a, b))
	case "i32x4_add":
		if !aok || !bok {
			return bad()
		}
		return one(v.I32x4Add(a, b))
	case "i64x2_add":
		if !aok || !bok {
			return bad()
		}
		return one(v.I64x2Add(a, b))
	case "i8x16_sub":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16Sub(a, b))
	case "i16x8_sub":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8Sub(a, b))
	case "i32x4_sub":
		if !aok || !bok {
			return bad()
		}
		return one(v.I32x4Sub(a, b))
	case "i64x2_sub":
		if !aok || !bok {
			return bad()
		}
		return one(v.I64x2Sub(a, b))
	case "i16x8_mul":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8Mul(a, b))
	case "i16x8_mulhi_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8MulHiS(a, b))
	case "i16x8_mulhi_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8MulHiU(a, b))
	case "i32x4_mul_even_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I32x4MulEvenU(a, b))
	case "i8x16_add_sat_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16AddSatS(a, b))
	case "i8x16_add_sat_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16AddSatU(a, b))
	case "i16x8_add_sat_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8AddSatS(a, b))
	case "i16x8_add_sat_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8AddSatU(a, b))
	case "i8x16_sub_sat_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16SubSatS(a, b))
	case "i8x16_sub_sat_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16SubSatU(a, b))
	case "i16x8_sub_sat_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8SubSatS(a, b))
	case "i16x8_sub_sat_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8SubSatU(a, b))
	case "i8x16_min_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16MinU(a, b))
	case "i8x16_max_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16MaxU(a, b))
	case "i16x8_min_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8MinS(a, b))
	case "i16x8_max_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8MaxS(a, b))
	case "i8x16_avgr_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16AvgrU(a, b))
	case "i16x8_avgr_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8AvgrU(a, b))
	case "i8x16_sad_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16SadU(a, b))
	case "i16x8_madd_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8MaddS(a, b))
	case "i8x16_eq":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16Eq(a, b))
	case "i16x8_eq":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8Eq(a, b))
	case "i32x4_eq":
		if !aok || !bok {
			return bad()
		}
		return one(v.I32x4Eq(a, b))
	case "i8x16_gt_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16GtS(a, b))
	case "i16x8_gt_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8GtS(a, b))
	case "i32x4_gt_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I32x4GtS(a, b))
	case "i8x16_unpack_low":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16UnpackLow(a, b))
	case "i8x16_unpack_high":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16UnpackHigh(a, b))
	case "i16x8_unpack_low":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8UnpackLow(a, b))
	case "i16x8_unpack_high":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8UnpackHigh(a, b))
	case "i32x4_unpack_low":
		if !aok || !bok {
			return bad()
		}
		return one(v.I32x4UnpackLow(a, b))
	case "i32x4_unpack_high":
		if !aok || !bok {
			return bad()
		}
		return one(v.I32x4UnpackHigh(a, b))
	case "i64x2_unpack_low":
		if !aok || !bok {
			return bad()
		}
		return one(v.I64x2UnpackLow(a, b))
	case "i64x2_unpack_high":
		if !aok || !bok {
			return bad()
		}
		return one(v.I64x2UnpackHigh(a, b))
	case "i8x16_narrow_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16NarrowS(a, b))
	case "i8x16_narrow_u":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16NarrowU(a, b))
	case "i16x8_narrow_s":
		if !aok || !bok {
			return bad()
		}
		return one(v.I16x8NarrowS(a, b))
	case "i8x16_swizzle":
		if !aok || !bok {
			return bad()
		}
		return one(v.I8x16Swizzle(a, b))
	case "aes_enc":
		if !aok || !bok {
			return bad()
		}
		return one(v.AesEnc(a, b))
	case "aes_enc_last":
		if !aok || !bok {
			return bad()
		}
		return one(v.AesEncLast(a, b))
	case "aes_dec":
		if !aok || !bok {
			return bad()
		}
		return one(v.AesDec(a, b))
	case "aes_dec_last":
		if !aok || !bok {
			return bad()
		}
		return one(v.AesDecLast(a, b))
	case "sha1_msg2":
		if !aok || !bok {
			return bad()
		}
		return one(v.Sha1Msg2(a, b))
	case "sha256_msg1":
		if !aok || !bok {
			return bad()
		}
		return one(v.Sha256Msg1(a, b))
	case "sha512_msg1":
		if !aok || !bok {
			return bad()
		}
		return one(v.Sha512Msg1(a, b))
	case "sm4_rounds":
		if !aok || !bok {
			return bad()
		}
		return one(v.Sm4Rounds(a, b))
	case "sm4_key":
		if !aok || !bok {
			return bad()
		}
		return one(v.Sm4Key(a, b))
	case "not":
		if !aok {
			return bad()
		}
		return one(v.Not(a))
	case "i16x8_bswap":
		if !aok {
			return bad()
		}
		return one(v.I16x8Bswap(a))
	case "i32x4_bswap":
		if !aok {
			return bad()
		}
		return one(v.I32x4Bswap(a))
	case "i64x2_bswap":
		if !aok {
			return bad()
		}
		return one(v.I64x2Bswap(a))
	case "aes_inv_mix_columns":
		if !aok {
			return bad()
		}
		return one(v.AesInvMixColumns(a))
	case "xor3":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Xor3(a, b, d))
	case "xor_andnot":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.XorAndNot(a, b, d))
	case "sha1_msg1":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sha1Msg1(a, b, d))
	case "sha256_rounds_abcd":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sha256RoundsAbcd(a, b, d))
	case "sha256_rounds_efgh":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sha256RoundsEfgh(a, b, d))
	case "sha256_msg2":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sha256Msg2(a, b, d))
	case "sha512_rounds_1":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sha512Rounds1(a, b, d))
	case "sha512_rounds_2":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sha512Rounds2(a, b, d))
	case "sha512_msg2":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sha512Msg2(a, b, d))
	case "sm3_ss1":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sm3Ss1(a, b, d))
	case "sm3_partw1":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sm3Partw1(a, b, d))
	case "sm3_partw2":
		if !aok || !bok || !dok {
			return bad()
		}
		return one(v.Sm3Partw2(a, b, d))
	case "shl_bytes":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.ShlBytes(a, k))
	case "shr_bytes":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.ShrBytes(a, k))
	case "i32x4_rotl":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.I32x4Rotl(a, k))
	case "i64x2_rotl":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.I64x2Rotl(a, k))
	case "i32x4_shuffle":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.I32x4Shuffle(a, k))
	case "i16x8_shuffle_low":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.I16x8ShuffleLow(a, k))
	case "i16x8_shuffle_high":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.I16x8ShuffleHigh(a, k))
	case "aes_keygen_assist":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.AesKeygenAssist(a, k))
	case "zext_i32":
		x, ok := args[0].(ir.I32)
		if !ok {
			return bad()
		}
		return one(v.ZExtI32(x))
	case "zext_i64":
		x, ok := args[0].(ir.I64)
		if !ok {
			return bad()
		}
		return one(v.ZExtI64(x))
	case "i16x8_shl":
		n, nok := args[1].(ir.I32)
		if !aok || !nok {
			return bad()
		}
		return one(v.I16x8Shl(a, n))
	case "i32x4_shl":
		n, nok := args[1].(ir.I32)
		if !aok || !nok {
			return bad()
		}
		return one(v.I32x4Shl(a, n))
	case "i64x2_shl":
		n, nok := args[1].(ir.I32)
		if !aok || !nok {
			return bad()
		}
		return one(v.I64x2Shl(a, n))
	case "i16x8_shr_u":
		n, nok := args[1].(ir.I32)
		if !aok || !nok {
			return bad()
		}
		return one(v.I16x8ShrU(a, n))
	case "i32x4_shr_u":
		n, nok := args[1].(ir.I32)
		if !aok || !nok {
			return bad()
		}
		return one(v.I32x4ShrU(a, n))
	case "i64x2_shr_u":
		n, nok := args[1].(ir.I32)
		if !aok || !nok {
			return bad()
		}
		return one(v.I64x2ShrU(a, n))
	case "i16x8_shr_s":
		n, nok := args[1].(ir.I32)
		if !aok || !nok {
			return bad()
		}
		return one(v.I16x8ShrS(a, n))
	case "i32x4_shr_s":
		n, nok := args[1].(ir.I32)
		if !aok || !nok {
			return bad()
		}
		return one(v.I32x4ShrS(a, n))
	case "i8x16_bitmask":
		if !aok {
			return bad()
		}
		return one(v.I8x16Bitmask(a))
	case "i8x16_splat":
		x, ok := args[0].(ir.I32)
		if !ok {
			return bad()
		}
		return one(v.I8x16Splat(x))
	case "i16x8_splat":
		x, ok := args[0].(ir.I32)
		if !ok {
			return bad()
		}
		return one(v.I16x8Splat(x))
	case "i32x4_splat":
		x, ok := args[0].(ir.I32)
		if !ok {
			return bad()
		}
		return one(v.I32x4Splat(x))
	case "i64x2_splat":
		x, ok := args[0].(ir.I64)
		if !ok {
			return bad()
		}
		return one(v.I64x2Splat(x))
	case "i16x8_extract_lane_u":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.I16x8ExtractLaneU(a, k))
	case "i16x8_replace_lane":
		x, xok := args[1].(ir.I32)
		k, ok, err := lit(2)
		if !aok || !xok || !ok {
			return nil, true, err
		}
		return one(v.I16x8ReplaceLane(a, x, k))
	case "i32x4_extract_lane":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.I32x4ExtractLane(a, k))
	case "i64x2_extract_lane":
		k, ok, err := lit(1)
		if !aok || !ok {
			return nil, true, err
		}
		return one(v.I64x2ExtractLane(a, k))
	case "i64x2_clmul":
		k, ok, err := lit(2)
		if !aok || !bok || !ok {
			return nil, true, err
		}
		return one(v.I64x2Clmul(a, b, k))
	case "sha1_rounds_ch":
		e, eok := args[1].(ir.I32)
		if !aok || !eok || !dok {
			return bad()
		}
		return one(v.Sha1RoundsCh(a, e, d))
	case "sha1_rounds_parity":
		e, eok := args[1].(ir.I32)
		if !aok || !eok || !dok {
			return bad()
		}
		return one(v.Sha1RoundsParity(a, e, d))
	case "sha1_rounds_maj":
		e, eok := args[1].(ir.I32)
		if !aok || !eok || !dok {
			return bad()
		}
		return one(v.Sha1RoundsMaj(a, e, d))
	case "sm3_tt1a":
		k, ok, err := lit(3)
		if !aok || !bok || !dok || !ok {
			return nil, true, err
		}
		return one(v.Sm3Tt1a(a, b, d, k))
	case "sm3_tt1b":
		k, ok, err := lit(3)
		if !aok || !bok || !dok || !ok {
			return nil, true, err
		}
		return one(v.Sm3Tt1b(a, b, d, k))
	case "sm3_tt2a":
		k, ok, err := lit(3)
		if !aok || !bok || !dok || !ok {
			return nil, true, err
		}
		return one(v.Sm3Tt2a(a, b, d, k))
	case "sm3_tt2b":
		k, ok, err := lit(3)
		if !aok || !bok || !dok || !ok {
			return nil, true, err
		}
		return one(v.Sm3Tt2b(a, b, d, k))
	}
	return nil, true, c.fail(ErrBuiltin, "builtin", name)
}
