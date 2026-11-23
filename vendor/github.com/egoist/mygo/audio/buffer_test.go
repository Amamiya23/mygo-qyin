package audio

import (
	"io"
	"testing"
	"testing/synctest"
	"time"
)

const bufferTestRate = 4 * decodeFrames

type bufferTestDecoder struct {
	blocks chan error
	seeked chan int64
}

func (d *bufferTestDecoder) SampleRate() int { return bufferTestRate }
func (d *bufferTestDecoder) Channels() int   { return 2 }
func (d *bufferTestDecoder) Len() int64      { return -1 }
func (d *bufferTestDecoder) SetPosition(at int64) error {
	d.seeked <- at
	return nil
}

func (d *bufferTestDecoder) Read(dst []float32) (int, error) {
	err, open := <-d.blocks
	if !open {
		return 0, io.EOF
	}
	if err != nil {
		return 0, err
	}
	for i := range dst {
		dst[i] = 0.5
	}
	return len(dst), nil
}

func (d *bufferTestDecoder) feed(seconds int) {
	for range 4 * seconds {
		d.blocks <- nil
	}
	synctest.Wait()
}

func TestStreamBufferWatermarks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := &bufferTestDecoder{make(chan error, 32), make(chan int64, 1)}
		p := NewBufferedPlayer(d, StreamBuffer{Capacity: 8 * time.Second, Start: 5 * time.Second, Resume: 6 * time.Second})
		defer close(d.blocks)
		defer p.Close()
		p.v.playing = true
		out := make([]float32, 6*bufferTestRate*2)
		mix := func(wantFrames bool, seconds int) {
			t.Helper()
			n, active := p.v.play(out[:seconds*bufferTestRate*2], 1, bufferTestRate)
			if (n > 0) != wantFrames || !active {
				t.Fatalf("mixed %d frames, active=%v, want frames=%v", n, active, wantFrames)
			}
		}
		d.feed(1)
		mix(false, 1)
		d.feed(3)
		mix(false, 1)
		d.feed(1)
		mix(true, 1)
		if p.Buffering() {
			t.Fatal("still buffering after five seconds")
		}
		mix(true, 6)
		if !p.Buffering() {
			t.Fatal("did not buffer after running dry")
		}
		d.feed(1)
		mix(false, 1)
		d.feed(4)
		mix(false, 1)
		p.Pause()
		d.feed(1)
		if n, active := p.v.play(out, 1, bufferTestRate); n != 0 || active {
			t.Fatalf("pause during buffering mixed %d frames, active=%v", n, active)
		}
		p.v.playing = true
		mix(true, 1)
		if p.Buffering() {
			t.Fatal("still buffering after six seconds")
		}

		p.Seek(10 * time.Second)
		d.feed(0)
		// Release the decoder read from before the seek before adding new data.
		d.blocks <- nil
		if at := <-d.seeked; at != 10*bufferTestRate {
			t.Fatalf("seeked to frame %d", at)
		}
		d.feed(4)
		mix(false, 1)
		d.feed(1)
		mix(true, 1)
	})
}

func TestStreamBufferEndsBelowWatermark(t *testing.T) {
	for _, err := range []error{io.EOF, io.ErrUnexpectedEOF} {
		t.Run(err.Error(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				d := &bufferTestDecoder{blocks: make(chan error, 8)}
				p := NewBufferedPlayer(d, StreamBuffer{Start: 5 * time.Second})
				defer p.Close()
				p.v.playing = true
				d.feed(1)
				d.blocks <- err
				synctest.Wait()
				out := make([]float32, 2*bufferTestRate*2)
				n, active := p.v.play(out, 1, bufferTestRate)
				var wantErr error
				wantFrames := bufferTestRate
				if err != io.EOF {
					wantErr, wantFrames = err, 0
				}
				if n != wantFrames || active || p.Buffering() || p.StreamError() != wantErr {
					t.Fatalf("mixed %d frames, active=%v, buffering=%v, error=%v", n, active, p.Buffering(), p.StreamError())
				}
			})
		})
	}
}
