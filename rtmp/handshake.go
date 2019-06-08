package rtmp

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"github.com/q191201771/lal/bele"
	"github.com/q191201771/lal/log"
	"io"
	"time"
)

// TODO chef: doc

// TODO chef: HandshakeClient with complex mode

const version = uint8(3)

const (
	c2Len     = 1536
	s1Len     = 1536
	s2Len     = 1536
	c0c1Len   = 1537
	s0s1Len   = 1537
	s0s1s2Len = 3073
)

const (
	clientPartKeyLen = 30
	serverPartKeyLen = 36
	serverFullKeyLen = 68
	keyLen           = 32
)

var serverVersion = []byte{
	0x0D, 0x0E, 0x0A, 0x0D,
}

// 30+32
var clientKey = []byte{
	'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
	'F', 'l', 'a', 's', 'h', ' ', 'P', 'l', 'a', 'y', 'e', 'r', ' ',
	'0', '0', '1',

	0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
	0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
	0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
}

// 36+32
var serverKey = []byte{
	'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
	'F', 'l', 'a', 's', 'h', ' ', 'M', 'e', 'd', 'i', 'a', ' ',
	'S', 'e', 'r', 'v', 'e', 'r', ' ',
	'0', '0', '1',

	0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
	0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
	0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
}

var random1528Buf []byte

type HandshakeClient struct {
	c0c1 []byte
	c2   []byte
}

type HandshakeServer struct {
	isSimpleMode bool
	s0s1s2       []byte
}

func (c *HandshakeClient) WriteC0C1(writer io.Writer) error {
	c.c0c1 = make([]byte, c0c1Len)
	c.c0c1[0] = version
	bele.BEPutUint32(c.c0c1[1:5], uint32(time.Now().UnixNano()))

	random1528(c.c0c1[9:])

	_, err := writer.Write(c.c0c1)
	log.Infof("<----- Handshake C0+C1")
	return err
}

func (c *HandshakeClient) ReadS0S1S2(reader io.Reader) error {
	s0s1s2 := make([]byte, s0s1s2Len)
	if _, err := io.ReadAtLeast(reader, s0s1s2, s0s1s2Len); err != nil {
		return err
	}
	log.Infof("-----> Handshake S0+S1+S2")
	if s0s1s2[0] != version {
		return rtmpErr
	}
	// use s2 as c2
	c.c2 = append(c.c2, s0s1s2[s0s1Len:]...)

	return nil
}

func (c *HandshakeClient) WriteC2(write io.Writer) error {
	_, err := write.Write(c.c2)
	log.Infof("<----- Handshake C2")
	return err
}

func (s *HandshakeServer) ReadC0C1(reader io.Reader) (err error) {
	c0c1 := make([]byte, c0c1Len)
	if _, err = io.ReadAtLeast(reader, c0c1, c0c1Len); err != nil {
		return err
	}
	log.Infof("-----> Handshake C0+C1")

	s.s0s1s2 = make([]byte, s0s1s2Len)

	var s2key []byte
	if s2key, err = parseChallenge(c0c1); err != nil {
		return err
	}
	s.isSimpleMode = len(s2key) == 0

	c1ts := bele.BEUint32(c0c1[1:])
	now := uint32(time.Now().UnixNano())

	s.s0s1s2[0] = version

	s1 := s.s0s1s2[1:]
	s2 := s.s0s1s2[s0s1Len:]

	bele.BEPutUint32(s1, now)
	random1528(s1[8:])

	bele.BEPutUint32(s2, c1ts)
	bele.BEPutUint32(s2[4:], now)
	random1528(s2[8:])

	if s.isSimpleMode {
		// s1
		bele.BEPutUint32(s1[4:], 0)

		//copy(s.s0s1s2, c0c1)
		//copy(s.s0s1s2[s0s1Len:], c0c1)
	} else {
		// s1
		copy(s1[4:], serverVersion)

		offs := int(s1[8]) + int(s1[9]) + int(s1[10]) + int(s1[11])
		offs = (offs % 728) + 12
		makeDigestWithoutCenterPart(s.s0s1s2[1:1+s1Len], offs, serverKey[:serverPartKeyLen], s.s0s1s2[1+offs:])

		// s2
		// make digest to s2 suffix position
		replyOffs := s2Len - keyLen
		makeDigestWithoutCenterPart(s2, replyOffs, s2key, s2[replyOffs:])
	}

	return nil
}

func (s *HandshakeServer) WriteS0S1S2(write io.Writer) error {
	_, err := write.Write(s.s0s1s2)
	log.Infof("<----- Handshake S0S1S2")
	return err
}

func (s *HandshakeServer) ReadC2(reader io.Reader) error {
	c2 := make([]byte, c2Len)
	if _, err := io.ReadAtLeast(reader, c2, c2Len); err != nil {
		return err
	}
	log.Infof("-----> Handshake C2")
	return nil
}

func parseChallenge(c0c1 []byte) ([]byte, error) {
	if c0c1[0] != version {
		return nil, rtmpErr
	}
	ver := bele.BEUint32(c0c1[5:])
	if ver == 0 {
		log.Debug("handshake simple mode.")
		return nil, nil
	}

	offs := findDigest(c0c1[1:], 764+8, clientKey[:clientPartKeyLen])
	if offs == -1 {
		offs = findDigest(c0c1[1:], 8, clientKey[:clientPartKeyLen])
	}
	if offs == -1 {
		log.Warn("get digest offs failed. roll back to try simple handshake.")
		return nil, nil
	}
	log.Debug("handshake complex mode.")

	// use c0c1 digest to make a new digest
	digest := makeDigest(c0c1[1+offs:1+offs+keyLen], serverKey[:serverFullKeyLen])

	return digest, nil
}

func findDigest(c1 []byte, base int, key []byte) int {
	// calc offs
	offs := int(c1[base]) + int(c1[base+1]) + int(c1[base+2]) + int(c1[base+3])
	offs = (offs % 728) + base + 4
	// calc digest
	digest := make([]byte, keyLen)
	makeDigestWithoutCenterPart(c1, offs, key, digest)
	// compare origin digest in buffer with calced digest
	if bytes.Compare(digest, c1[offs:offs+keyLen]) == 0 {
		return offs
	}
	return -1
}

// <b> could be `c1` or `s1` or `s2`
func makeDigestWithoutCenterPart(b []byte, offs int, key []byte, out []byte) {
	mac := hmac.New(sha256.New, key)
	// left
	if offs != 0 {
		mac.Write(b[:offs])
	}
	// right
	if len(b)-offs-keyLen > 0 {
		mac.Write(b[offs+keyLen:])
	}
	// calc
	copy(out, mac.Sum(nil))
}

func makeDigest(b []byte, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(b)
	return mac.Sum(nil)
}

func random1528(out []byte) {
	copy(out, random1528Buf)
}

func init() {
	// TODO chef: hack lal in
	bs := []byte{'l', 'a', 'l'}
	bsl := len(bs)
	random1528Buf = make([]byte, 1528)
	for i := 0; i < 1528; i++ {
		random1528Buf[i] = bs[i%bsl]
	}
}