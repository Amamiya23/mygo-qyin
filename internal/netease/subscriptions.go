package netease

import (
	"fmt"
	"strconv"
)

// SubscribePodcast subscribes the listener to a podcast, or unsubscribes
// them.
func SubscribePodcast(id int64, subscribe bool) error {
	return subscription("djradio", subscribe, map[string]any{"id": strconv.FormatInt(id, 10)})
}

// FollowArtist follows an artist for the listener, or unfollows them.
func FollowArtist(id int64, follow bool) error {
	return subscription("artist", follow, map[string]any{"artistId": strconv.FormatInt(id, 10), "artistIds": fmt.Sprintf("[%d]", id)})
}

func subscription(kind string, sub bool, data map[string]any) error {
	path := "/api/" + kind + "/unsub"
	if sub {
		path = "/api/" + kind + "/sub"
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := eapiPost(path, data, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: %s: code %d %s", path, res.Code, res.Message)
	}
	return nil
}
