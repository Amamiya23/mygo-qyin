package player

import (
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/cmplx"
	"net/http"
	"os"
	"sync"

	"github.com/egoist/mygo/audio"
	gomp3 "github.com/hajimehoshi/go-mp3"

	"lilt/internal/network"
	"lilt/internal/qqmusic"
)

// A download fetches a song as it plays: readers wait for the bytes they
// need, so playback starts with the first ones. The song comes into a file
// of the cache, which keeps it once it has all come in, and is read from
// there: the system holds what of it was read lately, rather than the
// app's memory, as much again as the song. Songs the cache would not keep
// come into memory.
type download struct {
	mu     sync.Mutex
	cond   sync.Cond
	file   *os.File // where the song is, or nil: then data
	data   []byte
	n      int64 // the bytes in so far
	size   int64 // expected, or 0 if unknown
	done   bool
	err    error
	cancel context.CancelFunc
	// part is the file's path until the cache keeps it, which keep does
	// once the song has all come in: the path it is at then.
	part string
	keep func(part string, n int64) (string, bool)
	// users counts what reads or writes the song: the stream decoding it,
	// until it closes, and the goroutine fetching it. The last to go
	// closes the file, and removes it unless the cache kept it.
	users  int
	closed bool
}

var streamHTTP = &http.Client{Transport: network.Transport()}

// startDownload fetches url, of size bytes if known, into part, a file
// that keep keeps once it has all come in: or into memory, without one.
func startDownload(url string, size int64, part *os.File, keep func(string, int64) (string, bool)) *download {
	ctx, cancel := context.WithCancel(context.Background())
	d := &download{size: size, cancel: cancel, users: 2}
	d.cond.L = &d.mu
	switch {
	case part != nil:
		d.file, d.part, d.keep = part, part.Name(), keep
	case size > 0:
		d.data = make([]byte, 0, prealloc(size))
	}
	go d.run(ctx, url)
	return d
}

// maxPrealloc is the most memory set aside for a song before its bytes
// come in: the size it is said to be is the API's word, which a song that
// turns out longer only outgrows.
const maxPrealloc = 32 << 20

func prealloc(size int64) int64 { return min(size, maxPrealloc) }

// downloaded returns a download that has all of b in already.
func downloaded(b []byte) *download {
	d := &download{data: b, n: int64(len(b)), size: int64(len(b)), done: true, cancel: func() {}, users: 1}
	d.cond.L = &d.mu
	return d
}

// opened returns a download that has all of the file f in already, of
// size bytes, as a song from the cache.
func opened(f *os.File, size int64) *download {
	d := &download{file: f, n: size, size: size, done: true, cancel: func() {}, users: 1}
	d.cond.L = &d.mu
	return d
}

func (d *download) run(ctx context.Context, url string) {
	defer d.release()
	finish := func(err error) {
		d.mu.Lock()
		d.done, d.err = true, err
		d.cond.Broadcast()
		d.mu.Unlock()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		finish(err)
		return
	}
	req.Header.Set("User-Agent", qqmusic.UserAgent)
	res, err := streamHTTP.Do(req)
	if err != nil {
		finish(err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		finish(errors.New(res.Status))
		return
	}
	buf := make([]byte, 64<<10)
	for {
		n, err := res.Body.Read(buf)
		if n > 0 {
			d.write(buf[:n])
		}
		if err == io.EOF {
			// Whole if as long as promised: the cache's.
			if d.part != "" && (d.size <= 0 || d.n == d.size) {
				d.hand()
			}
			finish(nil)
			return
		}
		if err != nil {
			finish(err)
			return
		}
	}
}

// write adds b to what is in. Only run writes, so it reads the file and
// what is in without the lock, which guards them from readers.
func (d *download) write(b []byte) {
	if d.file != nil {
		if _, err := d.file.Write(b); err != nil {
			d.toMemory()
		}
	}
	d.mu.Lock()
	if d.file == nil {
		d.data = append(d.data, b...)
	}
	d.n += int64(len(b))
	d.cond.Broadcast()
	d.mu.Unlock()
}

// toMemory moves the song into memory from a file that takes no more, as
// on a full disk: it plays on, but is not kept.
func (d *download) toMemory() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.data = make([]byte, d.n, max(prealloc(d.size), d.n))
	got, _ := d.file.ReadAt(d.data, 0)
	d.data = d.data[:got]
	d.n = int64(got)
	d.file.Close()
	os.Remove(d.part)
	d.file, d.part = nil, ""
}

// hand gives the song, all in, to the cache, and reads it from where it
// is kept. Windows moves no file that is open: it closes the file first,
// and opens it again where it went, while no one reads it.
func (d *download) hand() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.file.Close()
	path, ok := d.keep(d.part, d.n)
	f, err := os.Open(path)
	if err != nil {
		d.file, d.n, d.done, d.err = nil, 0, true, err
		return
	}
	d.file = f
	if ok {
		d.part = ""
	}
}

// Close stops the download and wakes its readers: the stream lets the
// song go.
func (d *download) Close() {
	d.cancel()
	d.mu.Lock()
	if !d.done {
		d.done, d.err = true, context.Canceled
	}
	d.cond.Broadcast()
	closed := d.closed
	d.closed = true
	d.mu.Unlock()
	if !closed {
		d.release()
	}
}

// share counts one more user of the song, as a second stream decoding
// it, unless it is gone already.
func (d *download) share() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.users <= 0 {
		return false
	}
	d.users++
	return true
}

// release lets the song go for one of its users. With none left, nothing
// reads it any more: its file closes, and goes unless the cache kept it.
func (d *download) release() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.users--; d.users > 0 {
		return
	}
	if d.file != nil {
		d.file.Close()
	}
	if d.part != "" {
		os.Remove(d.part)
	}
	d.file, d.data, d.n, d.part = nil, nil, 0, ""
}

// Progress returns the share of the song downloaded so far.
func (d *download) Progress() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.done && d.err == nil {
		return 1
	}
	if d.size <= 0 {
		return 0
	}
	return float64(d.n) / float64(d.size)
}

// waitFor waits until n bytes are in, or the download ended, and returns
// how many there are.
func (d *download) waitFor(n int64) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	for d.n < n && !d.done {
		d.cond.Wait()
	}
	return d.n
}

// readAt reads into b what is in from off, as much as fits. Its caller
// holds the lock.
func (d *download) readAt(b []byte, off int64) (int, error) {
	n := min(int64(len(b)), d.n-off)
	if n <= 0 {
		return 0, nil
	}
	if d.file == nil {
		return copy(b, d.data[off:d.n]), nil
	}
	return d.file.ReadAt(b[:n], off)
}

// peek returns up to n bytes from off, as far as they are in.
func (d *download) peek(off int64, n int) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	b := make([]byte, n)
	got, _ := d.readAt(b, off)
	return b[:got]
}

// A window reads a download a piece at a time, for scanning it byte by
// byte without reading the file for each.
type window struct {
	dl  *download
	off int64
	b   []byte
}

// at returns up to n bytes from off, as far as they are in.
func (w *window) at(off int64, n int) []byte {
	if off < w.off || off+int64(n) > w.off+int64(len(w.b)) {
		w.off, w.b = off, w.dl.peek(off, max(n, 32<<10))
	}
	return w.b[off-w.off : min(int64(len(w.b)), off-w.off+int64(n))]
}

// A downloadReader reads a download from an offset, waiting for its bytes.
// It is no io.Seeker, so go-mp3 does not read the whole file up front to
// find where each frame starts.
type downloadReader struct {
	d   *download
	pos int64
}

func (r *downloadReader) Read(p []byte) (int, error) {
	d := r.d
	d.mu.Lock()
	defer d.mu.Unlock()
	for d.n <= r.pos && !d.done {
		d.cond.Wait()
	}
	if d.n <= r.pos {
		if d.err != nil {
			return 0, d.err
		}
		return 0, io.EOF
	}
	n, err := d.readAt(p, r.pos)
	if n == 0 {
		return 0, cmp.Or(err, io.ErrUnexpectedEOF)
	}
	r.pos += int64(n)
	return n, nil
}

// A stream decodes a download for package audio, and measures the
// spectrum of what it decodes, for the visualizers: an MP3's or a FLAC's.
type stream interface {
	audio.Decoder
	Close() error
	// source returns the download decoded, and its spectrum.
	source() (*download, *spectrum)
}

// openStream opens the song dl fetches, MP3 or FLAC, as its first bytes
// say. An MP3 says nothing of its length: durationMs is it.
func openStream(dl *download, durationMs int64) (stream, error) {
	return openStreamMeasured(dl, durationMs, true)
}

func openStreamMeasured(dl *download, durationMs int64, measure bool) (stream, error) {
	off := id3Size(dl)
	if dl.waitFor(off+4) >= off+4 && string(dl.peek(off, 4)) == "fLaC" {
		return openFLACStream(dl, measure)
	}
	return openMP3StreamMeasured(dl, durationMs, measure)
}

// openShared opens a second stream of what dl fetches, for decoding apart
// from what plays: it measures no spectrum, and closing it lets the song
// go for it alone, leaving the download to the stream that plays.
func openShared(dl *download, durationMs int64) (stream, error) {
	if !dl.share() {
		return nil, errors.New("player: the song is gone")
	}
	s, err := openStreamMeasured(dl, durationMs, false)
	if err != nil {
		dl.release()
		return nil, err
	}
	switch s := s.(type) {
	case *mp3Stream:
		s.drop = dl.release
	case *flacStream:
		s.drop = dl.release
	}
	return s, nil
}

// mp3Stream decodes a download for package audio, seeking by bytes: songs
// are encoded at a constant bit rate, so time and bytes are in proportion.
// It also measures the spectrum of what it decodes, for the visualizers.
type mp3Stream struct {
	dl      *download
	dec     *gomp3.Decoder
	rate    int
	frames  int64 // the song's length in frames, from its duration
	start   int64 // the first audio frame's byte, after the ID3 tag
	pos     int64 // the frame decoded next
	skip    int   // bytes of decoded audio to drop after a seek
	buf     []byte
	spec    *spectrum
	drop    func() // lets the download go, as the stream closes
	closeMu sync.Once
}

func openMP3Stream(dl *download, durationMs int64) (*mp3Stream, error) {
	return openMP3StreamMeasured(dl, durationMs, true)
}

func openMP3StreamMeasured(dl *download, durationMs int64, measure bool) (*mp3Stream, error) {
	s := &mp3Stream{dl: dl, drop: dl.Close}
	dec, err := gomp3.NewDecoder(&downloadReader{d: dl})
	if err != nil {
		return nil, err
	}
	s.dec, s.rate = dec, dec.SampleRate()
	s.frames = durationMs * int64(s.rate) / 1000
	s.start = id3Size(dl)
	if measure {
		s.spec = newSpectrum(s.rate)
	}
	return s, nil
}

// id3Size returns the length of the ID3v2 tag at the start of the file.
func id3Size(dl *download) int64 {
	dl.waitFor(10)
	b := dl.peek(0, 10)
	if len(b) < 10 || string(b[:3]) != "ID3" {
		return 0
	}
	size := int64(b[6]&0x7f)<<21 | int64(b[7]&0x7f)<<14 | int64(b[8]&0x7f)<<7 | int64(b[9]&0x7f)
	return 10 + size
}

func (s *mp3Stream) SampleRate() int { return s.rate }
func (s *mp3Stream) Channels() int   { return 2 }
func (s *mp3Stream) Len() int64      { return s.frames }

func (s *mp3Stream) Read(dst []float32) (int, error) {
	n := len(dst) / 2 * 4
	if cap(s.buf) < n {
		s.buf = make([]byte, n)
	}
	b := s.buf[:n]
	for s.skip > 0 {
		got, err := io.ReadFull(s.dec, b[:min(n, s.skip)])
		s.skip -= got
		if err != nil {
			s.skip = 0
		}
	}
	got, err := io.ReadFull(s.dec, b)
	got -= got % 4
	samples := got / 2
	for i := range samples {
		dst[i] = float32(int16(binary.LittleEndian.Uint16(b[2*i:]))) / (1 << 15)
	}
	frames := int64(samples / 2)
	s.spec.feed(s.pos, dst[:samples])
	s.pos += frames
	if samples > 0 {
		return samples, nil
	}
	if err == io.ErrUnexpectedEOF || err == nil {
		err = io.EOF
	}
	return 0, err
}

// preroll is how many frames of sound a seek decodes before the one asked
// for, and drops. An MP3 frame borrows bits from the frames before it, up
// to 511 bytes back, which at 128 kbps is two frames, and its sound
// overlaps theirs: the first few decoded from the middle of a file are
// noise, as loud as the loudest sound.
const preroll = 4 * 1152

func (s *mp3Stream) SetPosition(frame int64) error {
	var off int64
	if from := frame - preroll; from > 0 && s.frames > 0 {
		size := s.dl.size
		if size <= 0 {
			size = s.dl.waitFor(math.MaxInt64)
		}
		off = s.start + int64(float64(size-s.start)*float64(from)/float64(s.frames))
		off = findFrame(s.dl, off)
	}
	dec, err := gomp3.NewDecoder(&downloadReader{d: s.dl, pos: off})
	if err != nil {
		return err
	}
	s.dec, s.pos = dec, frame
	s.spec.seek()
	// Drop what comes before frame: the preroll, or near the start, all
	// of it from there.
	s.skip = int(min(frame, preroll)) * 4
	return nil
}

func (s *mp3Stream) source() (*download, *spectrum) { return s.dl, s.spec }

func (s *mp3Stream) Close() error {
	s.closeMu.Do(s.drop)
	return nil
}

// findFrame returns the start of the first MP3 frame at or after off whose
// header the next frame's confirms, so that bytes that only look like one
// within the audio do not count.
func findFrame(dl *download, off int64) int64 {
	w := window{dl: dl}
	for tries := 0; tries < 64<<10; tries++ {
		have := dl.waitFor(off + 4)
		if have < off+4 {
			return off
		}
		size := frameSize(w.at(off, 4))
		if size > 0 {
			dl.waitFor(off + int64(size) + 4)
			next := w.at(off+int64(size), 4)
			if len(next) < 4 || frameSize(next) > 0 {
				return off
			}
		}
		off++
	}
	return off
}

var (
	bitratesV1 = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	bitratesV2 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	rates      = [4]int{44100, 48000, 32000, 0}
)

// frameSize returns the length of the MPEG layer III frame whose header b
// starts with, or 0 if it starts with none.
func frameSize(b []byte) int {
	if len(b) < 4 || b[0] != 0xff || b[1]&0xe0 != 0xe0 {
		return 0
	}
	version := (b[1] >> 3) & 3 // 3: MPEG 1, 2: MPEG 2, 0: MPEG 2.5
	layer := (b[1] >> 1) & 3   // 1: layer III
	if version == 1 || layer != 1 {
		return 0
	}
	bi, ri := b[2]>>4, (b[2]>>2)&3
	if bi == 0 || bi == 15 || ri == 3 {
		return 0
	}
	pad := int(b[2]>>1) & 1
	rate := rates[ri]
	switch version {
	case 3:
		return 144*bitratesV1[bi]*1000/rate + pad
	case 2:
		return 72*bitratesV2[bi]*1000/(rate/2) + pad
	default:
		return 72*bitratesV2[bi]*1000/(rate/4) + pad
	}
}

// The spectrum keeps only recent mono PCM. Visualizers request the chunk
// currently heard; a worker measures that chunk once, outside the audio and
// UI threads. With no visualizer asking, no FFT work is done.

const (
	fftSize = 1024
	Bands   = 24
	// MyGo decodes a second ahead and holds another 250 ms in the output.
	// Two seconds plus four chunks cover both, independently of song length.
	// The cap covers even the largest 20-bit FLAC sample rate's predecode
	// and output delay, while keeping the slots below 8.3 MiB at any rate.
	maxSpectrumChunks = 2048
)

type spectrumChunk struct {
	at       int64
	pcm      [fftSize]float32
	bands    [Bands]float32
	analyzed bool
}

type spectrum struct {
	mu     sync.Mutex
	chunks []spectrumChunk
	// requested holds only the latest chunk asked for. A worker exits when
	// it has caught up; it never polls, holds a timer or measures the ring.
	requested int64
	working   bool
	epoch     uint64
	lastAt    int64
	last      [Bands]float32
	maxLag    int64
	window    [fftSize]float32
	acc       []float32 // mono samples of the chunk being filled
	accAt     int64     // its first frame
	edges     [Bands + 1]int
}

func newSpectrum(rate int) *spectrum {
	n := min(maxSpectrumChunks, max(8, rate*2/fftSize+4))
	s := &spectrum{chunks: make([]spectrumChunk, n), accAt: -1, requested: -1,
		lastAt: -1, maxLag: max(4, int64(rate)/fftSize/10+1)}
	for i := range s.chunks {
		s.chunks[i].at = -1
	}
	for i := range s.window {
		s.window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)))
	}
	// Bands spaced evenly in pitch, from 40 Hz to 14 kHz.
	lo, hi := math.Log(40), math.Log(14000)
	for i := range s.edges {
		f := math.Exp(lo + (hi-lo)*float64(i)/Bands)
		s.edges[i] = max(1, min(fftSize/2-1, int(f*fftSize/float64(rate))))
	}
	for i := 1; i < len(s.edges); i++ {
		s.edges[i] = max(s.edges[i], s.edges[i-1]+1)
	}
	return s
}

// feed takes stereo samples decoded from frame at, unless the stream
// measures no spectrum.
func (s *spectrum) feed(at int64, stereo []float32) {
	if s == nil {
		return
	}
	if s.accAt < 0 || at != s.accAt+int64(len(s.acc)) {
		// The start, or a seek: begin a chunk at a chunk's boundary.
		s.acc = s.acc[:0]
		skip := (fftSize - at%fftSize) % fftSize
		if int(skip)*2 >= len(stereo) {
			s.accAt = -1
			return
		}
		stereo = stereo[skip*2:]
		at += skip
		s.accAt = at
	}
	for i := 0; i+1 < len(stereo); i += 2 {
		s.acc = append(s.acc, (stereo[i]+stereo[i+1])/2)
		if len(s.acc) == fftSize {
			s.keep(s.accAt / fftSize)
			s.accAt += fftSize
			s.acc = s.acc[:0]
		}
	}
}

func (s *spectrum) keep(chunk int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := &s.chunks[chunk%int64(len(s.chunks))]
	if entry.at == chunk {
		return
	}
	entry.at, entry.analyzed = chunk, false
	copy(entry.pcm[:], s.acc)
}

// seek drops pending display results while preserving reusable immutable PCM.
func (s *spectrum) seek() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.epoch++
	s.requested, s.lastAt = -1, -1
	s.mu.Unlock()
}

func (s *spectrum) analyze(pcm *[fftSize]float32) [Bands]float32 {
	var transformed [fftSize]complex128
	for i, v := range pcm {
		transformed[i] = complex(float64(v*s.window[i]), 0)
	}
	fft(transformed[:])
	var bands [Bands]float32
	for b := range Bands {
		var sum float64
		for k := s.edges[b]; k < s.edges[b+1]; k++ {
			sum += cmplx.Abs(transformed[k])
		}
		mag := sum / float64(s.edges[b+1]-s.edges[b]) / (fftSize / 4)
		// Higher bands carry less energy: tilt them up, as the ear hears.
		mag *= 1 + float64(b)/6
		db := 20 * math.Log10(mag+1e-9)
		bands[b] = float32(math.Max(0, math.Min(1, (db+60)/52)))
	}
	return bands
}

// at returns the bands of frame, requesting them if they are still unmeasured.
func (s *spectrum) at(frame int64) ([Bands]float32, bool) {
	if s == nil || frame < 0 {
		return [Bands]float32{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := frame / fftSize
	// A 30 Hz visualizer often advances to another chunk before its worker
	// finishes. Show the recent result while the current chunk is measured,
	// but never carry it across a seek or a gap longer than about 100 ms.
	var recent [Bands]float32
	if s.lastAt >= 0 && s.lastAt <= c && c-s.lastAt <= s.maxLag {
		recent = s.last
	}
	entry := &s.chunks[c%int64(len(s.chunks))]
	if entry.at != c {
		return recent, false
	}
	if entry.analyzed {
		s.lastAt, s.last = c, entry.bands
		return entry.bands, true
	}
	s.requested = c
	if !s.working {
		s.working = true
		go s.measure()
	}
	return recent, false
}

func (s *spectrum) measure() {
	for {
		s.mu.Lock()
		c := s.requested
		if c < 0 {
			s.working = false
			s.mu.Unlock()
			return
		}
		s.requested = -1
		epoch := s.epoch
		entry := &s.chunks[c%int64(len(s.chunks))]
		if entry.at != c || entry.analyzed {
			s.mu.Unlock()
			continue
		}
		pcm := entry.pcm
		s.mu.Unlock()
		bands := s.analyze(&pcm)
		s.mu.Lock()
		// A seek or forward decoding may have replaced this slot meanwhile.
		if entry.at == c && s.epoch == epoch {
			entry.bands, entry.analyzed = bands, true
			s.lastAt, s.last = c, bands
		}
		s.mu.Unlock()
	}
}

// fft transforms x in place; len(x) is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], x[start+k+size/2]*wk
				x[start+k], x[start+k+size/2] = a+b, a-b
				wk *= w
			}
		}
	}
}
