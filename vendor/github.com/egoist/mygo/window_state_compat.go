package mygo

import "encoding/json"

// UnmarshalJSON also reads the physical bounds saved by Lilt's earlier
// mixed-DPI fix. Subsequent saves use MyGo's pixel fields.
func (st *WindowState) UnmarshalJSON(data []byte) error {
	type state WindowState
	var saved struct {
		state
		WindowsBounds *Rectangle `json:"windowsBounds"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	if b := saved.WindowsBounds; b != nil && saved.PixelWidth <= 0 && saved.PixelHeight <= 0 {
		saved.PixelX, saved.PixelY = b.X, b.Y
		saved.PixelWidth, saved.PixelHeight = b.Width, b.Height
	}
	*st = WindowState(saved.state)
	return nil
}
