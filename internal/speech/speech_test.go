package speech

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// wavSource reads 16-bit PCM from a WAV file, as a Source.
type wavSource struct {
	r        io.Reader
	rate, ch int
}

func openWAV(t *testing.T, path string) *wavSource {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	head := make([]byte, 12)
	io.ReadFull(f, head)
	s := &wavSource{r: f}
	for {
		var chunk [8]byte
		if _, err := io.ReadFull(f, chunk[:]); err != nil {
			t.Fatal("wav: no data")
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		switch string(chunk[:4]) {
		case "fmt ":
			b := make([]byte, size)
			io.ReadFull(f, b)
			s.ch = int(binary.LittleEndian.Uint16(b[2:]))
			s.rate = int(binary.LittleEndian.Uint32(b[4:]))
		case "data":
			return s
		default:
			f.Seek(size, io.SeekCurrent)
		}
	}
}

func (s *wavSource) SampleRate() int { return s.rate }
func (s *wavSource) Channels() int   { return s.ch }
func (s *wavSource) Read(dst []float32) (int, error) {
	b := make([]byte, len(dst)*2)
	n, err := io.ReadFull(s.r, b)
	for i := range n / 2 {
		dst[i] = float32(int16(binary.LittleEndian.Uint16(b[2*i:]))) / (1 << 15)
	}
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n / 2, err
}

// A recording transcribes to what is said in it, in segments that follow
// one another from where it starts in the episode. It needs a WAV of
// Chinese speech in SPEECH_WAV, and SPEECH_ROOT, which Download fills with
// what is missing of the model and the libraries, from the app's release;
// or else the libraries and Silero's VAD in SHERPA_ONNX_DIR and the model
// in SPEECH_MODEL.
func TestTranscribe(t *testing.T) {
	root, dir, wav := os.Getenv("SPEECH_ROOT"), os.Getenv("SPEECH_MODEL"), os.Getenv("SPEECH_WAV")
	var r *Recognizer
	var err error
	switch {
	case wav == "":
		t.Skip("set SPEECH_WAV, and SPEECH_ROOT or SHERPA_ONNX_DIR and SPEECH_MODEL")
	case root != "":
		if err := Download(context.Background(), root, func(int64, int64) {}); err != nil {
			t.Fatal(err)
		}
		r, err = NewRecognizer(root)
	case dir != "" && os.Getenv("SHERPA_ONNX_DIR") != "":
		r, err = newRecognizer(dir, os.Getenv("SHERPA_ONNX_DIR"))
	default:
		t.Skip("set SPEECH_ROOT, or SHERPA_ONNX_DIR and SPEECH_MODEL")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	from := 10 * time.Minute
	var segs []Segment
	var heard time.Duration
	start := time.Now()
	err = r.Transcribe(context.Background(), openWAV(t, wav), from, from+time.Minute, func(s Segment) {
		t.Logf("%v–%v %s", s.Start, s.End, s.Text)
		segs = append(segs, s)
	}, func(d time.Duration) { heard = d })
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("a minute in %v", time.Since(start))
	if len(segs) == 0 {
		t.Fatal("nothing transcribed")
	}
	for i, s := range segs {
		if s.Start < from || s.End <= s.Start || i > 0 && s.Start < segs[i-1].End {
			t.Errorf("segment %d at %v–%v", i, s.Start, s.End)
		}
	}
	if heard != from+time.Minute {
		t.Errorf("heard up to %v, want %v", heard, from+time.Minute)
	}
	var text strings.Builder
	for _, s := range segs {
		text.WriteString(s.Text)
	}
	if !strings.ContainsAny(text.String(), "，。？") {
		t.Errorf("no punctuation in %q", text.String())
	}
}

// A segment's sentences become lines of their own, long ones cut at a
// comma, short ones kept with the line before, with no full stop at
// their ends.
func TestSplit(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"有些话题是绕不开的。比如说，我的婚姻结束了。", []string{"有些话题是绕不开的", "比如说，我的婚姻结束了"}},
		{"那一批的话，主要就不是说像前一批的这些商人啊，然后富甲一方的这些有影响力的人，多数都是出身于最底层的这些苦力。",
			[]string{"那一批的话，主要就不是说像前一批的这些商人啊", "然后富甲一方的这些有影响力的人", "多数都是出身于最底层的这些苦力"}},
		{"看不到它的关联的原因，但是它可能跟就是潮汕的发音是有关联的。哦。", []string{"看不到它的关联的原因，但是它可能跟就是潮汕的发音是有关联的。哦"}},
		{"那能不能就别去假装真？我们就探讨一下情吧。", []string{"那能不能就别去假装真？", "我们就探讨一下情吧"}},
		{"他说：“我要节食啊。”然后就走了。", []string{"他说：“我要节食啊。”", "然后就走了"}},
	} {
		var got []string
		for _, l := range Lines([]Segment{{Start: 0, End: 10 * time.Second, Text: c.in}}) {
			got = append(got, l.Text)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("Lines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A segment's lines share its time by how much each says, end to end.
func TestLinesTimed(t *testing.T) {
	seg := Segment{Start: 10 * time.Second, End: 16 * time.Second, Text: "有些话题是绕不开的。比如说，我的婚姻结束了。her edition。"}
	lines := Lines([]Segment{seg})
	if len(lines) != 3 {
		t.Fatalf("%d lines", len(lines))
	}
	if lines[0].At != seg.Start || lines[2].End != seg.End {
		t.Errorf("lines span %v–%v, want %v–%v", lines[0].At, lines[2].End, seg.Start, seg.End)
	}
	for i := 1; i < len(lines); i++ {
		if lines[i].At != lines[i-1].End {
			t.Errorf("line %d starts at %v, after %v", i, lines[i].At, lines[i-1].End)
		}
	}
	// 9, 10 and 4 syllables of 23.
	if d := lines[0].End - lines[0].At; math.Abs(d.Seconds()-6*9.0/23) > 0.01 {
		t.Errorf("first line lasts %v", d)
	}
	if d := lines[2].End - lines[2].At; math.Abs(d.Seconds()-6*4.0/23) > 0.01 {
		t.Errorf("last line lasts %v", d)
	}
}

// withModel makes the model files with these contents, served by the
// sources for the test, and a library, lib, gzipped, served at libURL.
func withModel(t *testing.T, files map[string][]byte, libURL string, sources ...func(modelFile) string) {
	oldFiles, oldSources, oldNatives, oldURL := modelFiles, modelSources, natives, nativeURL
	t.Cleanup(func() { modelFiles, modelSources, natives, nativeURL = oldFiles, oldSources, oldNatives, oldURL })
	modelFiles = nil
	for _, path := range []string{"model.onnx", "tokenizer/vocab.json"} {
		sum := sha256.Sum256(files[path])
		modelFiles = append(modelFiles, modelFile{path: path, size: int64(len(files[path])), sha256: hex.EncodeToString(sum[:]), from: "repo/" + path})
	}
	modelSources = sources
	gz := gzipped(lib)
	sum, gzSum := sha256.Sum256(lib), sha256.Sum256(gz)
	f := nativeFile{name: "libtest.dylib", asset: "lib.gz", size: int64(len(lib)), gzSize: int64(len(gz)), sha256: hex.EncodeToString(sum[:]), gzSHA256: hex.EncodeToString(gzSum[:])}
	natives = func() []nativeFile { return []nativeFile{f} }
	nativeURL = func(nativeFile) string { return libURL }
}

var lib = bytes.Repeat([]byte("\xcf\xfa\xed\xfe code "), 1000)

func gzipped(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

func serve(t *testing.T, h http.HandlerFunc) func(modelFile) string {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return func(f modelFile) string { return srv.URL + "/" + f.from }
}

// A source that fails gives way to the next, and a file that stopped
// halfway goes on from there, asking only for the rest.
func TestDownloadResumes(t *testing.T) {
	files := map[string][]byte{
		"model.onnx":           bytes.Repeat([]byte("onnx"), 50000),
		"tokenizer/vocab.json": []byte(`{"你":1}`),
	}
	var ranges []string
	libURL := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(gzipped(lib)) })(modelFile{from: "lib.gz"})
	withModel(t, files, libURL,
		serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusBadGateway) }),
		serve(t, func(w http.ResponseWriter, r *http.Request) {
			ranges = append(ranges, r.Header.Get("Range"))
			b := files[strings.TrimPrefix(r.URL.Path, "/repo/")]
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b))
		}))
	root := t.TempDir()
	dir := filepath.Join(root, ModelName)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "model.onnx.part"), files["model.onnx"][:120000], 0o644)
	var last, total int64
	if err := Download(context.Background(), root, func(d, t int64) { last, total = d, t }); err != nil {
		t.Fatal(err)
	}
	if !Installed(root) {
		t.Fatal("not installed")
	}
	if got, _ := os.ReadFile(filepath.Join(libDir(root), "libtest.dylib")); !bytes.Equal(got, lib) {
		t.Error("the library differs")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "model.onnx")); !bytes.Equal(got, files["model.onnx"]) {
		t.Error("model.onnx differs")
	}
	if ranges[0] != "bytes=120000-" || ranges[1] != "" {
		t.Errorf("asked for %q", ranges)
	}
	if last != total || total != 200009+int64(len(gzipped(lib))) {
		t.Errorf("progress %d of %d", last, total)
	}
}

// A file that comes in other than the model's is thrown away, not kept
// as it, nor gone on from.
func TestDownloadChecksDigest(t *testing.T) {
	files := map[string][]byte{"model.onnx": []byte("model"), "tokenizer/vocab.json": []byte("{}")}
	evil := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("evil!")) })
	withModel(t, files, evil(modelFile{from: "lib.gz"}), evil)
	dir := t.TempDir()
	err := Download(context.Background(), dir, func(int64, int64) {})
	if !errors.Is(err, errDigest) {
		t.Fatalf("err = %v", err)
	}
	for _, name := range []string{"libtest.dylib", "libtest.dylib.gz", "libtest.dylib.gz.part"} {
		if _, err := os.Stat(filepath.Join(libDir(dir), name)); err == nil {
			t.Errorf("%s kept", name)
		}
	}
	if Installed(dir) {
		t.Error("installed")
	}
}

// A library changed after it downloaded, as by whatever else could write
// there, does not load into the app.
func TestChangedLibraryRefused(t *testing.T) {
	withModel(t, map[string][]byte{}, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "libtest.dylib")
	os.WriteFile(path, lib, 0o755)
	if err := verifyLibs(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, append(lib, "evil"...), 0o755)
	if verifyLibs(dir) == nil {
		t.Error("a changed library loads")
	}
}

// tone is seconds of a sine of freq Hz at rate, in stereo.
func tone(freq float64, rate int, seconds float64) []float32 {
	n := int(seconds * float64(rate))
	out := make([]float32, 2*n)
	for i := range n {
		v := float32(0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
		out[2*i], out[2*i+1] = v, v
	}
	return out
}

// rms is the loudness of x, but for its first and last tenth, where the
// filter starts and stops.
func rms(x []float32) float64 {
	x = x[len(x)/10 : len(x)*9/10]
	var sum float64
	for _, v := range x {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(len(x)))
}

// A song's rate comes down to the models' 16 kHz in mono: speech's
// frequencies as loud as they were, those above 8 kHz gone rather than
// folded down among them.
func TestResample(t *testing.T) {
	for _, rate := range []int{44100, 48000, 16000} {
		out := newResampler(rate, 2).push(nil, tone(1000, rate, 1))
		if n := len(out); math.Abs(float64(n-SampleRate)) > 50 {
			t.Errorf("%d Hz: %d samples for a second", rate, n)
		}
		if got := rms(out); math.Abs(got-0.5/math.Sqrt2) > 0.01 {
			t.Errorf("%d Hz: a 1 kHz tone at %.3f, want %.3f", rate, got, 0.5/math.Sqrt2)
		}
		if rate == SampleRate {
			continue
		}
		if got := rms(newResampler(rate, 2).push(nil, tone(12000, rate, 1))); got > 0.01 {
			t.Errorf("%d Hz: a 12 kHz tone folds down at %.3f", rate, got)
		}
	}
}

// Audio resamples the same in chunks as at once, as it decodes.
func TestResampleChunks(t *testing.T) {
	in := tone(440, 44100, 0.5)
	whole := newResampler(44100, 2).push(nil, in)
	r := newResampler(44100, 2)
	var parts []float32
	for i := 0; i < len(in); i += 2 * 1001 {
		parts = r.push(parts, in[i:min(i+2*1001, len(in))])
	}
	if len(parts) != len(whole) {
		t.Fatalf("%d samples in chunks, %d at once", len(parts), len(whole))
	}
	for i := range whole {
		if math.Abs(float64(parts[i]-whole[i])) > 1e-6 {
			t.Fatalf("sample %d: %v in chunks, %v at once", i, parts[i], whole[i])
		}
	}
}
