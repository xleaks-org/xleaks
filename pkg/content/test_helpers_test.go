package content

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

func testPNGWithDimensions(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})

	writeChunk := func(name string, data []byte) {
		_ = binary.Write(&buf, binary.BigEndian, uint32(len(data)))
		buf.WriteString(name)
		buf.Write(data)
		crc := crc32.ChecksumIEEE(append([]byte(name), data...))
		_ = binary.Write(&buf, binary.BigEndian, crc)
	}

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 2
	writeChunk("IHDR", ihdr)
	writeChunk("IEND", nil)

	return buf.Bytes()
}
