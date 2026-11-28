//go:build !ios

package audio

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// macOS plays through an Audio Queue on the default output, which follows
// it when it changes.

type audioStreamBasicDescription struct {
	sampleRate       float64
	formatID         uint32
	formatFlags      uint32
	bytesPerPacket   uint32
	framesPerPacket  uint32
	bytesPerFrame    uint32
	channelsPerFrame uint32
	bitsPerChannel   uint32
	reserved         uint32
}

type audioQueueBuffer struct {
	audioDataBytesCapacity uint32
	audioData              unsafe.Pointer
	audioDataByteSize      uint32
	userData               uintptr
	packetDescCapacity     uint32
	packetDescs            uintptr
	packetDescCount        uint32
}

const (
	kAudioFormatLinearPCM   = 0x6C70636D // 'lpcm'
	kAudioFormatFlagIsFloat = 1
	aqFrames                = 512 // per buffer: 10.7 ms, as many as hold ahead
	dispatchTimeNow         = 0
)

var (
	aqLoad sync.Once
	aqErr  error

	audioQueueNewOutput      func(format *audioStreamBasicDescription, callback uintptr, user uintptr, runLoop, runLoopMode uintptr, flags uint32, out *uintptr) int32
	audioQueueAllocateBuffer func(q uintptr, size uint32, out **audioQueueBuffer) int32
	audioQueueStart          func(q uintptr, at uintptr) int32
	audioQueueStop           func(q uintptr, immediate bool) int32
	audioQueueDispose        func(q uintptr, immediate bool) int32
	audioQueueGetProperty    func(q uintptr, id uint32, data *uint32, size *uint32) int32
	dispatchSemaphoreCreate  func(value int) uintptr
	dispatchRelease          func(obj uintptr)

	// Called on every buffer, through a cCaller.
	audioQueueEnqueueBuffer uintptr
	dispatchSemaphoreWait   uintptr
	dispatchTime            uintptr

	dispatchSemaphoreSignal uintptr // every queue's callback
)

func loadAudioToolbox() error {
	aqLoad.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			aqErr = err
			return
		}
		purego.RegisterLibFunc(&audioQueueNewOutput, lib, "AudioQueueNewOutput")
		purego.RegisterLibFunc(&audioQueueAllocateBuffer, lib, "AudioQueueAllocateBuffer")
		purego.RegisterLibFunc(&audioQueueStart, lib, "AudioQueueStart")
		purego.RegisterLibFunc(&audioQueueStop, lib, "AudioQueueStop")
		purego.RegisterLibFunc(&audioQueueDispose, lib, "AudioQueueDispose")
		purego.RegisterLibFunc(&audioQueueGetProperty, lib, "AudioQueueGetProperty")
		purego.RegisterLibFunc(&dispatchSemaphoreCreate, purego.RTLD_DEFAULT, "dispatch_semaphore_create")
		purego.RegisterLibFunc(&dispatchRelease, purego.RTLD_DEFAULT, "dispatch_release")
		audioQueueEnqueueBuffer = cFunc(lib, "AudioQueueEnqueueBuffer")
		dispatchSemaphoreWait = cFunc(purego.RTLD_DEFAULT, "dispatch_semaphore_wait")
		dispatchSemaphoreSignal = cFunc(purego.RTLD_DEFAULT, "dispatch_semaphore_signal")
		dispatchTime = cFunc(purego.RTLD_DEFAULT, "dispatch_time")
	})
	return aqErr
}

// An aqDevice is an Audio Queue whose callback is
// dispatch_semaphore_signal, its semaphore the user data (C on arm64 and
// x86-64 ignores the queue and buffer passed after it), so no Go runs on
// the queue's thread. The queue plays buffers in order: each signal frees
// the oldest.
type aqDevice struct {
	q       uintptr
	sema    uintptr
	bufs    []*audioQueueBuffer
	next    int // the buffer written next, the oldest enqueued once all are
	queued  int // buffers enqueued before the queue started
	started bool
	c       cCaller
}

func openDevice(ahead time.Duration, rate int) (device, int, error) {
	if err := loadAudioToolbox(); err != nil {
		return nil, 0, err
	}
	desc := audioStreamBasicDescription{
		sampleRate:       float64(rate),
		formatID:         kAudioFormatLinearPCM,
		formatFlags:      kAudioFormatFlagIsFloat,
		bytesPerPacket:   channels * 4,
		framesPerPacket:  1,
		bytesPerFrame:    channels * 4,
		channelsPerFrame: channels,
		bitsPerChannel:   32,
	}
	n := max(2, int((int64(ahead)*int64(rate)/int64(time.Second)+aqFrames-1)/aqFrames))
	// Fails only for a negative value.
	d := &aqDevice{sema: dispatchSemaphoreCreate(0), bufs: make([]*audioQueueBuffer, n)}
	if st := audioQueueNewOutput(&desc, dispatchSemaphoreSignal, d.sema, 0, 0, 0, &d.q); st != 0 {
		dispatchRelease(d.sema)
		return nil, 0, fmt.Errorf("audio: AudioQueueNewOutput failed (OSStatus %d)", st)
	}
	for i := range d.bufs {
		if st := audioQueueAllocateBuffer(d.q, aqFrames*channels*4, &d.bufs[i]); st != 0 {
			d.close()
			return nil, 0, fmt.Errorf("audio: AudioQueueAllocateBuffer failed (OSStatus %d)", st)
		}
	}
	return d, aqFrames, nil
}

func (d *aqDevice) write(samples []float32) error {
	if d.started {
		// Wait for the oldest buffer to play.
		deadline := d.c.call(dispatchTime, dispatchTimeNow, uintptr(time.Second))
		if d.c.call(dispatchSemaphoreWait, d.sema, deadline) != 0 {
			// The queue stopped taking buffers, as when its device went.
			return fmt.Errorf("audio: the audio queue stopped")
		}
	}
	b := d.bufs[d.next]
	d.next = (d.next + 1) % len(d.bufs)
	copy(unsafe.Slice((*float32)(b.audioData), len(samples)), samples)
	b.audioDataByteSize = uint32(len(samples) * 4)
	// b is C memory, as a cCaller needs.
	if st := int32(d.c.call(audioQueueEnqueueBuffer, d.q, uintptr(unsafe.Pointer(b)), 0, 0)); st != 0 {
		return fmt.Errorf("audio: AudioQueueEnqueueBuffer failed (OSStatus %d)", st)
	}
	if !d.started {
		// Start once every buffer holds something, not to begin with an
		// underrun.
		if d.queued++; d.queued == len(d.bufs) {
			if st := audioQueueStart(d.q, 0); st != 0 {
				return fmt.Errorf("audio: AudioQueueStart failed (OSStatus %d)", st)
			}
			d.started = true
		}
	}
	return nil
}

func (d *aqDevice) close() {
	// Synchronous: no callback runs once it returns.
	audioQueueStop(d.q, true)
	audioQueueDispose(d.q, true)
	// Never below its initial value, which dispatch_release requires.
	dispatchRelease(d.sema)
}

// drain confirms that Stop(false) played the queued tail. Stop may also stop
// the audio hardware, so request a fresh session and its configured pre-roll.
// Buffer reuse callbacks alone do not mean samples have played.
func (d *aqDevice) drain() (bool, error) {
	if !d.started && d.queued > 0 {
		if st := audioQueueStart(d.q, 0); st != 0 {
			return false, fmt.Errorf("audio: AudioQueueStart failed (OSStatus %d)", st)
		}
		d.started = true
	}
	if st := audioQueueStop(d.q, false); st != 0 {
		return false, fmt.Errorf("audio: AudioQueueStop failed (OSStatus %d)", st)
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		var running uint32
		size := uint32(4)
		if st := audioQueueGetProperty(d.q, 0x6171726e, &running, &size); st != 0 {
			return false, fmt.Errorf("audio: AudioQueueGetProperty(IsRunning) failed (OSStatus %d)", st)
		}
		if running == 0 {
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			return false, fmt.Errorf("audio: audio queue did not stop after draining")
		}
	}
	d.started, d.queued = false, 0
	return true, nil
}
