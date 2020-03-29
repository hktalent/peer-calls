package tracks

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v2"
)

const (
	rtcpPLIInterval = time.Second * 3
)

type PeerConnection interface {
	AddTrack(*webrtc.Track) (*webrtc.RTPSender, error)
	RemoveTrack(*webrtc.RTPSender) error
	OnTrack(func(*webrtc.Track, *webrtc.RTPReceiver))
	OnICEConnectionStateChange(func(webrtc.ICEConnectionState))
	WriteRTCP([]rtcp.Packet) error
	NewTrack(uint8, uint32, string, string) (*webrtc.Track, error)
}

type Peer struct {
	clientID         string
	peerConnection   PeerConnection
	localTracks      []*webrtc.Track
	localTracksMu    sync.RWMutex
	rtpSenderByTrack map[*webrtc.Track]*webrtc.RTPSender
	onTrack          func(clientID string, track *webrtc.Track)
	onClose          func(clientID string)
}

func NewPeer(
	clientID string,
	peerConnection PeerConnection,
	onTrack func(clientID string, track *webrtc.Track),
	onClose func(clientID string),
) *Peer {
	p := &Peer{
		clientID:         clientID,
		peerConnection:   peerConnection,
		onTrack:          onTrack,
		onClose:          onClose,
		rtpSenderByTrack: map[*webrtc.Track]*webrtc.RTPSender{},
	}

	peerConnection.OnICEConnectionStateChange(p.handleICEConnectionStateChange)
	peerConnection.OnTrack(p.handleTrack)

	return p
}

// FIXME add support for data channel messages for sending chat messages, and images/files

func (p *Peer) ClientID() string {
	return p.clientID
}

func (p *Peer) AddTrack(track *webrtc.Track) error {
	rtpSender, err := p.peerConnection.AddTrack(track)
	if err != nil {
		return fmt.Errorf("Error adding track: %s to peer clientID: %s", track.ID(), p.clientID)
	}
	p.rtpSenderByTrack[track] = rtpSender
	return nil
}

func (p *Peer) RemoveTrack(track *webrtc.Track) error {
	rtpSender, ok := p.rtpSenderByTrack[track]
	if !ok {
		return fmt.Errorf("Cannot find sender for track: %s, clientID: %s", track.ID(), p.clientID)
	}
	return p.peerConnection.RemoveTrack(rtpSender)
}

func (p *Peer) handleICEConnectionStateChange(connectionState webrtc.ICEConnectionState) {
	log.Printf("Peer connection state changed, clientID: %s, state: %s",
		p.clientID,
		connectionState.String(),
	)
	if connectionState == webrtc.ICEConnectionStateClosed ||
		connectionState == webrtc.ICEConnectionStateDisconnected ||
		connectionState == webrtc.ICEConnectionStateFailed {
		// TODO prevent this method from being called twice (state disconnected, then failed)
		p.onClose(p.clientID)
	}
}

func (p *Peer) handleTrack(remoteTrack *webrtc.Track, receiver *webrtc.RTPReceiver) {
	log.Printf("handleTrack %s for clientID: %s", remoteTrack.ID(), p.clientID)
	localTrack, err := p.startCopyingTrack(remoteTrack)
	if err != nil {
		log.Printf("Error copying remote track: %s", err)
		return
	}
	p.localTracksMu.Lock()
	p.localTracks = append(p.localTracks, localTrack)
	p.localTracksMu.Unlock()

	p.onTrack(p.clientID, localTrack)
}

func (p *Peer) Tracks() []*webrtc.Track {
	return p.localTracks
}

func (p *Peer) startCopyingTrack(remoteTrack *webrtc.Track) (*webrtc.Track, error) {
	log.Printf("startCopyingTrack: %s for peer clientID: %s", remoteTrack.ID(), p.clientID)

	// Create a local track, all our SFU clients will be fed via this track
	localTrack, err := p.peerConnection.NewTrack(remoteTrack.PayloadType(), remoteTrack.SSRC(), "video", "pion")
	if err != nil {
		err = fmt.Errorf("startCopyingTrack: error creating new track, trackID: %s, clientID: %s, error: %s", remoteTrack.ID(), p.clientID, err)
		return nil, err
	}

	log.Printf(
		"startCopyingTrack: remote track %s to new local track: %s for clientID: %s",
		remoteTrack.ID(),
		localTrack.ID(),
		p.clientID,
	)

	// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
	// This can be less wasteful by processing incoming RTCP events, then we would emit a NACK/PLI when a viewer requests it

	ticker := time.NewTicker(rtcpPLIInterval)
	go func() {
		for range ticker.C {
			err := p.peerConnection.WriteRTCP(
				[]rtcp.Packet{
					&rtcp.PictureLossIndication{
						MediaSSRC: remoteTrack.SSRC(),
					},
				},
			)
			if err != nil {
				log.Printf("Error sending rtcp PLI for local track: %s for clientID: %s: %s",
					localTrack.ID(),
					p.clientID,
					err,
				)
			}
		}
	}()

	go func() {
		defer ticker.Stop()
		rtpBuf := make([]byte, 1400)
		for {
			i, err := remoteTrack.Read(rtpBuf)
			if err != nil {
				log.Printf(
					"Error reading from remote track: %s for clientID: %s: %s",
					remoteTrack.ID(),
					p.clientID,
					err,
				)
				return
			}

			// ErrClosedPipe means we don't have any subscribers, this is ok if no peers have connected yet
			if _, err = localTrack.Write(rtpBuf[:i]); err != nil && err != io.ErrClosedPipe {
				log.Printf(
					"Error writing to local track: %s for clientID: %s: %s",
					localTrack.ID(),
					p.clientID,
					err,
				)
				return
			}
		}
	}()

	return localTrack, nil
}