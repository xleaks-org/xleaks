package identity

import (
	"fmt"
	"strings"
)

const (
	addressHRP = "xleaks1"
	charset    = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
)

var charsetRev [128]int8

func init() {
	for i := range charsetRev {
		charsetRev[i] = -1
	}
	for i, c := range charset {
		charsetRev[c] = int8(i)
	}
}

func PubKeyToAddress(pubkey []byte) (string, error) {
	if len(pubkey) != 32 {
		return "", fmt.Errorf("public key must be 32 bytes, got %d", len(pubkey))
	}

	data, err := convertBits(pubkey, 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("failed to convert bits: %w", err)
	}

	return bech32Encode(addressHRP, data)
}

func AddressToPubKey(address string) ([]byte, error) {
	hrp, data, err := bech32Decode(address)
	if err != nil {
		return nil, fmt.Errorf("failed to decode bech32 address: %w", err)
	}

	if hrp != addressHRP {
		return nil, fmt.Errorf("invalid address prefix: expected %q, got %q", addressHRP, hrp)
	}

	decoded, err := convertBits(data, 5, 8, false)
	if err != nil {
		return nil, fmt.Errorf("failed to convert bits: %w", err)
	}

	if len(decoded) != 32 {
		return nil, fmt.Errorf("decoded public key must be 32 bytes, got %d", len(decoded))
	}

	return decoded, nil
}

func bech32Polymod(values []int) int {
	gen := [5]int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := 1
	for _, v := range values {
		b := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ v
		for i := 0; i < 5; i++ {
			if (b>>uint(i))&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func bech32HRPExpand(hrp string) []int {
	ret := make([]int, 0, len(hrp)*2+1)
	for _, c := range hrp {
		ret = append(ret, int(c>>5))
	}
	ret = append(ret, 0)
	for _, c := range hrp {
		ret = append(ret, int(c&31))
	}
	return ret
}

func bech32CreateChecksum(hrp string, data []int) []int {
	values := append(bech32HRPExpand(hrp), data...)
	values = append(values, 0, 0, 0, 0, 0, 0)
	polymod := bech32Polymod(values) ^ 1
	checksum := make([]int, 6)
	for i := 0; i < 6; i++ {
		checksum[i] = (polymod >> uint(5*(5-i))) & 31
	}
	return checksum
}

func bech32VerifyChecksum(hrp string, data []int) bool {
	return bech32Polymod(append(bech32HRPExpand(hrp), data...)) == 1
}

func bech32Encode(hrp string, data []byte) (string, error) {
	intData := make([]int, len(data))
	for i, b := range data {
		intData[i] = int(b)
	}

	checksum := bech32CreateChecksum(hrp, intData)
	combined := append(intData, checksum...)

	var sb strings.Builder
	sb.WriteString(hrp)
	for _, d := range combined {
		if d < 0 || d >= len(charset) {
			return "", fmt.Errorf("invalid data value: %d", d)
		}
		sb.WriteByte(charset[d])
	}
	return sb.String(), nil
}

func bech32Decode(bech string) (string, []byte, error) {
	if len(bech) > 90 {
		return "", nil, fmt.Errorf("bech32 string too long: %d > 90", len(bech))
	}

	for _, c := range bech {
		if c < 33 || c > 126 {
			return "", nil, fmt.Errorf("invalid character: %c", c)
		}
	}

	lower := strings.ToLower(bech)
	if lower != bech && strings.ToUpper(bech) != bech {
		return "", nil, fmt.Errorf("mixed case in bech32 string")
	}
	bech = lower

	// The HRP is everything before the data. In our encoding there's no
	// separator character -- the HRP is the known prefix "xleaks1".
	// For a general bech32 decoder the separator is the last '1' in the string.
	pos := strings.LastIndex(bech, "1")
	if pos < 1 || pos+7 > len(bech) {
		return "", nil, fmt.Errorf("invalid bech32 separator position")
	}

	hrp := bech[:pos+1]
	dataStr := bech[pos+1:]

	intData := make([]int, len(dataStr))
	for i, c := range dataStr {
		if c > 127 || charsetRev[c] == -1 {
			return "", nil, fmt.Errorf("invalid bech32 character: %c", c)
		}
		intData[i] = int(charsetRev[c])
	}

	if !bech32VerifyChecksum(hrp, intData) {
		return "", nil, fmt.Errorf("invalid bech32 checksum")
	}

	data := make([]byte, len(intData)-6)
	for i, v := range intData[:len(intData)-6] {
		data[i] = byte(v)
	}

	return hrp, data, nil
}

func convertBits(data []byte, fromBits, toBits uint, pad bool) ([]byte, error) {
	acc := 0
	bits := uint(0)
	maxv := (1 << toBits) - 1
	var ret []byte

	for _, b := range data {
		if int(b)>>fromBits != 0 {
			return nil, fmt.Errorf("invalid data value %d (exceeds %d bits)", b, fromBits)
		}
		acc = (acc << fromBits) | int(b)
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			ret = append(ret, byte((acc>>bits)&maxv))
		}
	}

	if pad {
		if bits > 0 {
			ret = append(ret, byte((acc<<(toBits-bits))&maxv))
		}
	} else {
		if bits >= fromBits {
			return nil, fmt.Errorf("invalid padding")
		}
		if (acc<<(toBits-bits))&maxv != 0 {
			return nil, fmt.Errorf("non-zero padding")
		}
	}

	return ret, nil
}
