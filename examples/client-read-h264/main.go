package main

import (
	"log"

	gortsplib "github.com/kuartis/gortsplib_go"
	"github.com/kuartis/gortsplib_go/pkg/base"
	"github.com/kuartis/gortsplib_go/pkg/rtph264"
	"github.com/kuartis/rtp"
)

// This example shows how to
// 1. connect to a RTSP server and read all tracks on a path
// 2. check if there's an H264 track
// 3. convert H264 into raw frames

// This example requires the ffmpeg libraries, that can be installed in this way:
// apt install -y libavformat-dev libswscale-dev gcc pkg-config

func main() {
	c := gortsplib.Client{}

	// parse URL
	u, err := base.ParseURL("rtsp://localhost:8554/mystream")
	if err != nil {
		panic(err)
	}

	// connect to the server
	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	// find published tracks
	tracks, baseURL, _, err := c.Describe(u)
	if err != nil {
		panic(err)
	}

	// find the H264 track
	h264tr, h264trID := func() (*gortsplib.TrackH264, int) {
		for i, track := range tracks {
			if h264tr, ok := track.(*gortsplib.TrackH264); ok {
				return h264tr, i
			}
		}
		return nil, -1
	}()
	if h264trID < 0 {
		panic("H264 track not found")
	}

	// setup RTP->H264 decoder
	rtpDec := rtph264.NewDecoder()

	// setup H264->raw frames decoder
	h264dec, err := newH264Decoder()
	if err != nil {
		panic(err)
	}
	defer h264dec.close()

	// send SPS and PPS to the decoder
	if h264tr.SPS() == nil || h264tr.PPS() == nil {
		panic("SPS or PPS not present into the SDP")
	}
	h264dec.decode(h264tr.SPS())
	h264dec.decode(h264tr.PPS())

	// called when a RTP packet arrives
	c.OnPacketRTP = func(trackID int, pkt *rtp.Packet) {
		if trackID != h264trID {
			return
		}

		// decode H264 NALUs from the RTP packet
		nalus, _, err := rtpDec.Decode(pkt)
		if err != nil {
			return
		}

		for _, nalu := range nalus {
			// decode raw frames from H264 NALUs
			img, err := h264dec.decode(nalu)
			if err != nil {
				panic(err)
			}

			// wait for a frame
			if img == nil {
				continue
			}

			log.Printf("decoded frame with size %v", img.Bounds().Max)
		}
	}

	// start reading tracks
	err = c.SetupAndPlay(tracks, baseURL)
	if err != nil {
		panic(err)
	}

	// wait until a fatal error
	panic(c.Wait())
}
