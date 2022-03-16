package rtpaac

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/kuartis/gortsplib_go/pkg/rtptimedec"
	"github.com/kuartis/rtp"
)

// ErrMorePacketsNeeded is returned when more packets are needed.
var ErrMorePacketsNeeded = errors.New("need more packets")

// Decoder is a RTP/AAC decoder.
type Decoder struct {
	timeDecoder          *rtptimedec.Decoder
	isDecodingFragmented bool
	fragmentedBuf        []byte
}

// NewDecoder allocates a Decoder.
func NewDecoder(clockRate int) *Decoder {
	return &Decoder{
		timeDecoder: rtptimedec.New(clockRate),
	}
}

// Decode decodes AUs from a RTP/AAC packet.
// It returns the AUs and the PTS of the first AU.
// The PTS of subsequent AUs can be calculated by adding time.Second*1000/clockRate.
func (d *Decoder) Decode(pkt *rtp.Packet) ([][]byte, time.Duration, error) {
	if len(pkt.Payload) < 2 {
		d.isDecodingFragmented = false
		return nil, 0, fmt.Errorf("payload is too short")
	}

	// AU-headers-length
	headersLen := binary.BigEndian.Uint16(pkt.Payload)
	if (headersLen % 16) != 0 {
		d.isDecodingFragmented = false
		return nil, 0, fmt.Errorf("invalid AU-headers-length (%d)", headersLen)
	}
	payload := pkt.Payload[2:]

	if !d.isDecodingFragmented {
		if pkt.Header.Marker {
			// AU-headers
			// AAC headers are 16 bits, where
			// * 13 bits are data size
			// * 3 bits are AU index
			headerCount := headersLen / 16
			var dataLens []uint16
			for i := 0; i < int(headerCount); i++ {
				if len(payload[i*2:]) < 2 {
					return nil, 0, fmt.Errorf("payload is too short")
				}

				header := binary.BigEndian.Uint16(payload[i*2:])
				dataLen := header >> 3
				auIndex := header & 0x03
				if auIndex != 0 {
					return nil, 0, fmt.Errorf("AU-index field is not zero")
				}

				dataLens = append(dataLens, dataLen)
			}
			payload = payload[headerCount*2:]

			// AUs
			aus := make([][]byte, len(dataLens))
			for i, dataLen := range dataLens {
				if len(payload) < int(dataLen) {
					return nil, 0, fmt.Errorf("payload is too short")
				}

				aus[i] = payload[:dataLen]
				payload = payload[dataLen:]
			}

			return aus, d.timeDecoder.Decode(pkt.Timestamp), nil
		}

		if headersLen != 16 {
			return nil, 0, fmt.Errorf("a fragmented packet can only contain one AU")
		}

		// AU-header
		header := binary.BigEndian.Uint16(payload)
		dataLen := header >> 3
		auIndex := header & 0x03
		if auIndex != 0 {
			return nil, 0, fmt.Errorf("AU-index field is not zero")
		}
		payload = payload[2:]

		if len(payload) < int(dataLen) {
			return nil, 0, fmt.Errorf("payload is too short")
		}

		d.fragmentedBuf = append(d.fragmentedBuf, payload...)

		d.isDecodingFragmented = true
		return nil, 0, ErrMorePacketsNeeded
	}

	// we are decoding a fragmented AU

	if headersLen != 16 {
		return nil, 0, fmt.Errorf("a fragmented packet can only contain one AU")
	}

	// AU-header
	header := binary.BigEndian.Uint16(payload)
	dataLen := header >> 3
	auIndex := header & 0x03
	if auIndex != 0 {
		return nil, 0, fmt.Errorf("AU-index field is not zero")
	}
	payload = payload[2:]

	if len(payload) < int(dataLen) {
		return nil, 0, fmt.Errorf("payload is too short")
	}

	d.fragmentedBuf = append(d.fragmentedBuf, payload...)

	if !pkt.Header.Marker {
		return nil, 0, ErrMorePacketsNeeded
	}

	d.isDecodingFragmented = false
	return [][]byte{d.fragmentedBuf}, d.timeDecoder.Decode(pkt.Timestamp), nil
}
