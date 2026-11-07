package audio

import (
	"errors"
	"fmt"
	"syscall/js"
	"time"
	"unsafe"
)

// Browsers play through an AudioWorklet, which asks the page for a chunk
// whenever it used one up. The AudioContext is recreated when its rate
// changes; closing the output otherwise suspends it. Pages may start audio only after the
// user interacted with them, so it resumes on the first input too.

const (
	jsFrames = 1024 // per chunk: 21 ms
	jsChunks = 3    // queued in the worklet to begin with, and at least
)

const worklet = `
class MyGoOutput extends AudioWorkletProcessor {
	constructor() {
		super();
		this.queue = [];
		this.offset = 0;
		this.port.onmessage = (e) => this.queue.push(e.data);
		for (let i = 0; i < %d; i++) this.port.postMessage(0);
	}
	process(inputs, outputs) {
		const left = outputs[0][0], right = outputs[0][1];
		let i = 0;
		while (i < left.length) {
			const chunk = this.queue[0];
			if (!chunk) {
				left.fill(0, i);
				right.fill(0, i);
				break;
			}
			const n = Math.min(left.length - i, chunk.length / 2 - this.offset);
			for (let j = 0; j < n; j++) {
				left[i + j] = chunk[2 * (this.offset + j)];
				right[i + j] = chunk[2 * (this.offset + j) + 1];
			}
			i += n;
			this.offset += n;
			if (this.offset * 2 >= chunk.length) {
				this.queue.shift();
				this.offset = 0;
				this.port.postMessage(0);
			}
		}
		return true;
	}
}
registerProcessor("mygo-output", MyGoOutput);
`

var (
	jsContext         js.Value
	jsPort            js.Value
	jsCredits         = make(chan struct{}, 32)
	jsErr             error
	jsInputRegistered bool
	jsMessageHandler  js.Func

	// jsAhead is how many chunks are queued in the worklet or asked for,
	// and jsOwed how many it asks for that are to go unanswered, as the
	// output holds less than it did.
	jsAhead = jsChunks
	jsOwed  int
)

// jsSetup creates an AudioContext at the requested rate. The worklet's
// queue follows latency changes without recreating the context.
func jsSetup(ahead time.Duration, rate int) error {
	if !jsContext.IsUndefined() && jsContext.Get("sampleRate").Int() != rate {
		jsPort.Set("onmessage", js.Null())
		jsMessageHandler.Release()
		jsContext.Call("close")
		jsContext = js.Undefined()
		jsAhead, jsOwed = jsChunks, 0
		for len(jsCredits) > 0 {
			<-jsCredits
		}
	}
	if !jsContext.IsUndefined() || jsErr != nil {
		return jsErr
	}
	class := js.Global().Get("AudioContext")
	if !class.Truthy() {
		jsErr = errors.New("audio: this browser has no AudioContext")
		return jsErr
	}
	hint := "interactive"
	if ahead >= LatencyPlayback.ahead() {
		hint = "playback"
	}
	ctx := class.New(map[string]any{"sampleRate": rate, "latencyHint": hint})
	if !ctx.Get("audioWorklet").Truthy() {
		jsErr = errors.New("audio: this browser has no AudioWorklet")
		return jsErr
	}
	blob := js.Global().Get("Blob").New([]any{fmt.Sprintf(worklet, jsChunks)}, map[string]any{"type": "text/javascript"})
	url := js.Global().Get("URL").Call("createObjectURL", blob)
	done := make(chan error, 1)
	var ok, fail js.Func
	ok = js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- nil
		return nil
	})
	fail = js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- fmt.Errorf("audio: loading the AudioWorklet failed: %s", js.Global().Get("String").Invoke(args[0]).String())
		return nil
	})
	ctx.Get("audioWorklet").Call("addModule", url).Call("then", ok, fail)
	err := <-done
	ok.Release()
	fail.Release()
	js.Global().Get("URL").Call("revokeObjectURL", url)
	if err != nil {
		jsErr = err
		return err
	}
	node := js.Global().Get("AudioWorkletNode").New(ctx, "mygo-output", map[string]any{
		"numberOfInputs":     0,
		"outputChannelCount": []any{channels},
	})
	node.Call("connect", ctx.Get("destination"))
	jsPort = node.Get("port")
	jsMessageHandler = js.FuncOf(func(this js.Value, args []js.Value) any {
		select {
		case jsCredits <- struct{}{}:
		default:
		}
		return nil
	})
	jsPort.Set("onmessage", jsMessageHandler)
	// The first input lets a page start audio.
	if !jsInputRegistered {
		jsInputRegistered = true
		resume := js.FuncOf(func(this js.Value, args []js.Value) any {
			if !jsContext.IsUndefined() && jsContext.Get("state").String() == "suspended" && jsOpen {
				jsContext.Call("resume")
			}
			return nil
		})
		for _, ev := range []string{"pointerdown", "keydown", "touchend"} {
			js.Global().Call("addEventListener", ev, resume, map[string]any{"capture": true})
		}
	}
	jsContext = ctx
	return nil
}

// jsOpen is whether the output is open, which keeps the context running.
var jsOpen bool

type jsDevice struct{}

func openDevice(ahead time.Duration, rate int) (device, int, error) {
	if err := jsSetup(ahead, rate); err != nil {
		return nil, 0, err
	}
	want := max(jsChunks, int((int64(ahead)*int64(rate)/int64(time.Second)+jsFrames-1)/jsFrames))
	if more := want - jsAhead; more > 0 {
		// Asks the owed ones answer first.
		cancel := min(jsOwed, more)
		jsOwed -= cancel
		for range more - cancel {
			jsCredits <- struct{}{}
		}
	} else {
		jsOwed -= more
	}
	jsAhead = want
	jsOpen = true
	jsContext.Call("resume")
	return jsDevice{}, jsFrames, nil
}

func (jsDevice) write(samples []float32) error {
	// A suspended context, until an input lets the page play, asks for
	// nothing: the output waits without mixing ahead.
	for ; jsOwed > 0; jsOwed-- {
		<-jsCredits
	}
	<-jsCredits
	a := js.Global().Get("Float32Array").New(len(samples))
	js.CopyBytesToJS(js.Global().Get("Uint8Array").New(a.Get("buffer")), unsafe.Slice((*byte)(unsafe.Pointer(&samples[0])), len(samples)*4))
	jsPort.Call("postMessage", a, []any{a.Get("buffer")})
	return nil
}

func (jsDevice) close() {
	jsOpen = false
	jsContext.Call("suspend")
}
