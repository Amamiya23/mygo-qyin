// Copyright 2010 The Go Authors. All rights reserved.
// The DES tables below derive from Go's crypto/des/const.go and are governed by
// its BSD license, reproduced in qrc_des.LICENSE. The decoder uses those standard
// FIPS 46-3 tables with the two QQ substitutions marked below.
package qqmusic

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/bits"
	"unicode/utf8"
)

// decodeQRC reads QQ's cloud lyric payload: hex -> modified DES-EDE -> zlib.
// QQ reverses bytes within 32-bit words, changes two S-box entries, and shifts
// PC-2's D-half selection, so crypto/des is not interchangeable. This is only a
// legacy protocol decoder.
// Protocol reference: https://github.com/lyswhut/lx-music-desktop/blob/master/src/renderer/utils/musicSdk/tx/qrcDecode.js
func decodeQRC(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	ciphertext, err := hex.DecodeString(s)
	if err != nil || len(ciphertext) == 0 || len(ciphertext)%8 != 0 {
		return "", fmt.Errorf("qqmusic: malformed QRC ciphertext")
	}
	for i := 0; i < len(ciphertext); i += 8 {
		block := qrcSwapWords(binary.BigEndian.Uint64(ciphertext[i:]))
		block = qrcCryptBlock(block, &qrcKeys[2], true)
		block = qrcCryptBlock(block, &qrcKeys[1], false)
		block = qrcCryptBlock(block, &qrcKeys[0], true)
		binary.BigEndian.PutUint64(ciphertext[i:], qrcSwapWords(block))
	}
	r, err := zlib.NewReader(bytes.NewReader(ciphertext))
	if err != nil {
		return "", fmt.Errorf("qqmusic: open QRC compression: %w", err)
	}
	defer r.Close()
	text, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("qqmusic: decompress QRC: %w", err)
	}
	if !utf8.Valid(text) {
		return "", fmt.Errorf("qqmusic: invalid QRC text encoding")
	}
	return string(text), nil
}

func qrcSwapWords(v uint64) uint64 {
	return uint64(bits.ReverseBytes32(uint32(v>>32)))<<32 | uint64(bits.ReverseBytes32(uint32(v)))
}

var qrcKeys = func() [3][16]uint64 {
	const key = "!@#)(*$%123ZXC!@!@#)(NHL"
	var keys [3][16]uint64
	for i := range keys {
		v := qrcPermute(qrcSwapWords(binary.BigEndian.Uint64([]byte(key[i*8:]))), qrcPC1[:])
		left, right := uint32(v>>28), uint32(v&0xfffffff)
		for round, shift := range qrcRotations {
			left = (left<<shift | left>>(28-shift)) & 0xfffffff
			right = (right<<shift | right>>(28-shift)) & 0xfffffff
			// QQ selects the D half one bit below standard PC-2. The source
			// uses a top-aligned 28-bit D with indices reduced by 27, not 28.
			var subkey uint64
			for j, pos := range qrcPC2 {
				var bit uint64
				if j < 24 {
					bit = uint64(left) >> (pos - 28) & 1
				} else if pos > 0 {
					bit = uint64(right) >> (pos - 1) & 1
				}
				subkey = subkey<<1 | bit
			}
			keys[i][round] = subkey
		}
	}
	return keys
}()

func qrcCryptBlock(block uint64, keys *[16]uint64, decrypt bool) uint64 {
	v := qrcPermute(block, qrcIP[:])
	left, right := uint32(v>>32), uint32(v)
	for round := range keys {
		key := round
		if decrypt {
			key = 15 - round
		}
		x := qrcPermute(uint64(right), qrcExpansion[:]) ^ keys[key]
		var substituted uint32
		for box := range qrcSBoxes {
			part := uint8(x >> uint(42-6*box) & 63)
			row := (part&32)>>4 | part&1
			column := part >> 1 & 15
			substituted = substituted<<4 | uint32(qrcSBoxes[box][row][column])
		}
		left, right = right, left^uint32(qrcPermute(uint64(substituted), qrcP[:]))
	}
	return qrcPermute(uint64(right)<<32|uint64(left), qrcFP[:])
}

// Table positions count from the least significant bit, starting at zero.
func qrcPermute(v uint64, positions []byte) uint64 {
	var result uint64
	for _, pos := range positions {
		result = result<<1 | v>>pos&1
	}
	return result
}

// Used to perform an initial permutation of a 64-bit input block.
var qrcIP = [64]byte{
	6, 14, 22, 30, 38, 46, 54, 62,
	4, 12, 20, 28, 36, 44, 52, 60,
	2, 10, 18, 26, 34, 42, 50, 58,
	0, 8, 16, 24, 32, 40, 48, 56,
	7, 15, 23, 31, 39, 47, 55, 63,
	5, 13, 21, 29, 37, 45, 53, 61,
	3, 11, 19, 27, 35, 43, 51, 59,
	1, 9, 17, 25, 33, 41, 49, 57,
}

// Used to perform a final permutation of a 64-bit preoutput block. This is the
// inverse of qrcIP
var qrcFP = [64]byte{
	24, 56, 16, 48, 8, 40, 0, 32,
	25, 57, 17, 49, 9, 41, 1, 33,
	26, 58, 18, 50, 10, 42, 2, 34,
	27, 59, 19, 51, 11, 43, 3, 35,
	28, 60, 20, 52, 12, 44, 4, 36,
	29, 61, 21, 53, 13, 45, 5, 37,
	30, 62, 22, 54, 14, 46, 6, 38,
	31, 63, 23, 55, 15, 47, 7, 39,
}

// Used to expand an input block of 32 bits, producing an output block of 48
// bits.
var qrcExpansion = [48]byte{
	0, 31, 30, 29, 28, 27, 28, 27,
	26, 25, 24, 23, 24, 23, 22, 21,
	20, 19, 20, 19, 18, 17, 16, 15,
	16, 15, 14, 13, 12, 11, 12, 11,
	10, 9, 8, 7, 8, 7, 6, 5,
	4, 3, 4, 3, 2, 1, 0, 31,
}

// Yields a 32-bit output from a 32-bit input
var qrcP = [32]byte{
	16, 25, 12, 11, 3, 20, 4, 15,
	31, 17, 9, 6, 27, 14, 1, 22,
	30, 24, 8, 18, 0, 5, 29, 23,
	13, 19, 2, 26, 10, 21, 28, 7,
}

// Used in the key schedule to select 56 bits
// from a 64-bit input.
var qrcPC1 = [56]byte{
	7, 15, 23, 31, 39, 47, 55, 63,
	6, 14, 22, 30, 38, 46, 54, 62,
	5, 13, 21, 29, 37, 45, 53, 61,
	4, 12, 20, 28, 1, 9, 17, 25,
	33, 41, 49, 57, 2, 10, 18, 26,
	34, 42, 50, 58, 3, 11, 19, 27,
	35, 43, 51, 59, 36, 44, 52, 60,
}

// Used in the key schedule to produce each subkey by selecting 48 bits from
// the 56-bit input
var qrcPC2 = [48]byte{
	42, 39, 45, 32, 55, 51, 53, 28,
	41, 50, 35, 46, 33, 37, 44, 52,
	30, 48, 40, 49, 29, 36, 43, 54,
	15, 4, 25, 19, 9, 1, 26, 16,
	5, 11, 23, 8, 12, 7, 17, 0,
	22, 3, 10, 14, 6, 20, 27, 24,
}

// 8 S-boxes composed of 4 rows and 16 columns
// Used in the DES cipher function
var qrcSBoxes = [8][4][16]uint8{
	// S-box 1
	{
		{14, 4, 13, 1, 2, 15, 11, 8, 3, 10, 6, 12, 5, 9, 0, 7},
		{0, 15, 7, 4, 14, 2, 13, 1, 10, 6, 12, 11, 9, 5, 3, 8},
		{4, 1, 14, 8, 13, 6, 2, 11, 15, 12, 9, 7, 3, 10, 5, 0},
		{15, 12, 8, 2, 4, 9, 1, 7, 5, 11, 3, 14, 10, 0, 6, 13},
	},
	// S-box 2
	{
		{15, 1, 8, 14, 6, 11, 3, 4, 9, 7, 2, 13, 12, 0, 5, 10},
		{3, 13, 4, 7, 15, 2, 8, 15, 12, 0, 1, 10, 6, 9, 11, 5}, // QQ: S2[23] = 15.
		{0, 14, 7, 11, 10, 4, 13, 1, 5, 8, 12, 6, 9, 3, 2, 15},
		{13, 8, 10, 1, 3, 15, 4, 2, 11, 6, 7, 12, 0, 5, 14, 9},
	},
	// S-box 3
	{
		{10, 0, 9, 14, 6, 3, 15, 5, 1, 13, 12, 7, 11, 4, 2, 8},
		{13, 7, 0, 9, 3, 4, 6, 10, 2, 8, 5, 14, 12, 11, 15, 1},
		{13, 6, 4, 9, 8, 15, 3, 0, 11, 1, 2, 12, 5, 10, 14, 7},
		{1, 10, 13, 0, 6, 9, 8, 7, 4, 15, 14, 3, 11, 5, 2, 12},
	},
	// S-box 4
	{
		{7, 13, 14, 3, 0, 6, 9, 10, 1, 2, 8, 5, 11, 12, 4, 15},
		{13, 8, 11, 5, 6, 15, 0, 3, 4, 7, 2, 12, 1, 10, 14, 9},
		{10, 6, 9, 0, 12, 11, 7, 13, 15, 1, 3, 14, 5, 2, 8, 4},
		{3, 15, 0, 6, 10, 10, 13, 8, 9, 4, 5, 11, 12, 7, 2, 14}, // QQ: S4[53] = 10.
	},
	// S-box 5
	{
		{2, 12, 4, 1, 7, 10, 11, 6, 8, 5, 3, 15, 13, 0, 14, 9},
		{14, 11, 2, 12, 4, 7, 13, 1, 5, 0, 15, 10, 3, 9, 8, 6},
		{4, 2, 1, 11, 10, 13, 7, 8, 15, 9, 12, 5, 6, 3, 0, 14},
		{11, 8, 12, 7, 1, 14, 2, 13, 6, 15, 0, 9, 10, 4, 5, 3},
	},
	// S-box 6
	{
		{12, 1, 10, 15, 9, 2, 6, 8, 0, 13, 3, 4, 14, 7, 5, 11},
		{10, 15, 4, 2, 7, 12, 9, 5, 6, 1, 13, 14, 0, 11, 3, 8},
		{9, 14, 15, 5, 2, 8, 12, 3, 7, 0, 4, 10, 1, 13, 11, 6},
		{4, 3, 2, 12, 9, 5, 15, 10, 11, 14, 1, 7, 6, 0, 8, 13},
	},
	// S-box 7
	{
		{4, 11, 2, 14, 15, 0, 8, 13, 3, 12, 9, 7, 5, 10, 6, 1},
		{13, 0, 11, 7, 4, 9, 1, 10, 14, 3, 5, 12, 2, 15, 8, 6},
		{1, 4, 11, 13, 12, 3, 7, 14, 10, 15, 6, 8, 0, 5, 9, 2},
		{6, 11, 13, 8, 1, 4, 10, 7, 9, 5, 0, 15, 14, 2, 3, 12},
	},
	// S-box 8
	{
		{13, 2, 8, 4, 6, 15, 11, 1, 10, 9, 3, 14, 5, 0, 12, 7},
		{1, 15, 13, 8, 10, 3, 7, 4, 12, 5, 6, 11, 0, 14, 9, 2},
		{7, 11, 4, 1, 9, 12, 14, 2, 0, 6, 10, 13, 15, 3, 5, 8},
		{2, 1, 14, 7, 4, 10, 8, 13, 15, 12, 9, 0, 3, 5, 6, 11},
	},
}

// Size of left rotation per round in each half of the key schedule
var qrcRotations = [16]uint8{1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1}
