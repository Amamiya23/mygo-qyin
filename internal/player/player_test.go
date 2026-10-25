package player

import "testing"

func TestPlayEmptyQueue(t *testing.T) {
	p := &Player{at: -1}
	p.PlayQueue(nil, 0) // an empty playlist's play button
	if p.Current() != nil {
		t.Error("playing from an empty queue")
	}
}
